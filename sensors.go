package amazondevices

import (
	"context"
	"fmt"
	"sync"

	"github.com/Fishwaldo/goamazondevices/internal/alexagraphql"
	"github.com/Fishwaldo/goamazondevices/internal/amzconst"
	"github.com/Fishwaldo/goamazondevices/internal/httpwrapper"
	"github.com/Khan/genqlient/graphql"
)

// SensorsService populates Device.Sensors (and rewrites Device.Online from
// the per-device reachability sensor) by POSTing the getEndpointState
// GraphQL operation to /nexus/v1/graphql.
//
// Wired into DevicesService.Refresh as the post-enrichment step. Callers
// don't usually invoke Update directly; it's called for them.
//
// Safe for concurrent use.
type SensorsService struct {
	wrapper *httpwrapper.Wrapper

	mu      sync.RWMutex
	session *Session
}

func (s *SensorsService) setSession(sess *Session) {
	s.mu.Lock()
	s.session = sess
	s.mu.Unlock()
}

// fetchEndpointStates POSTs the GetEndpointState GraphQL operation for
// every endpointID in endpoints and returns the typed response. Returns
// (nil, nil) if endpoints is empty so callers can short-circuit
// without an HTTP request.
func (s *SensorsService) fetchEndpointStates(
	ctx context.Context,
	endpoints map[string]string,
) (*alexagraphql.GetEndpointStateResponse, error) {
	if len(endpoints) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(endpoints))
	for id := range endpoints {
		ids = append(ids, id)
	}

	url := fmt.Sprintf("https://alexa.amazon.%s%s",
		s.wrapper.Domain(), amzconst.URINexusGraphQL)
	client := graphql.NewClient(url, alexagraphql.NewDoer(s.wrapper))
	resp, err := alexagraphql.GetEndpointState(ctx, client, ids)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCannotConnect, err)
	}
	return resp, nil
}

// Update fetches sensor state for the given endpoints and applies it to
// the devices map in place:
//
//   - Sets device.Sensors to the parsed sensor map (replacing any existing
//     entries). When no sensors are returned for a device, the existing
//     Sensors map is marked with Error: true on every entry.
//   - Sets device.Online from the "reachability" sensor's value ("OK" →
//     true, anything else → false). When no reachability sensor is
//     present, device.Online is set to false.
//   - For speaker-group devices (DeviceFamily == amzconst.SpeakerGroupFamily),
//     online is the AND of every cluster member's Online value.
//   - If dnd is non-nil, copies each matching DnD entry onto
//     device.Sensors["dnd"] for non-speaker-group devices.
//
// devices and endpoints are the maps populated by DevicesService earlier
// in the refresh pipeline (devices keyed by serial; endpoints keyed by
// endpointID → serial). Errors fetching the sensor state surface to the
// caller; per-device parse problems are logged at debug and don't fail
// the operation.
func (s *SensorsService) Update(
	ctx context.Context,
	devices map[string]*Device,
	endpoints map[string]string,
	dnd map[string]*DeviceSensor,
) error {
	resp, err := s.fetchEndpointStates(ctx, endpoints)
	if err != nil {
		return err
	}

	sensorsByDevice := s.parseSensorResponse(resp, endpoints, devices)

	for _, dvc := range devices {
		parsed := sensorsByDevice[dvc.SerialNumber]
		if len(parsed) > 0 {
			dvc.Sensors = parsed
			if reachable := parsed["reachability"]; reachable != nil {
				dvc.Online = reachable.Value == "OK"
			} else {
				dvc.Online = false
			}
		} else {
			dvc.Online = false
			for _, sensor := range dvc.Sensors {
				sensor.Error = true
			}
		}

		if dnd != nil {
			if entry := dnd[dvc.SerialNumber]; entry != nil &&
				dvc.DeviceFamily != amzconst.SpeakerGroupFamily {
				dvc.Sensors["dnd"] = entry
			}
		}

	}

	// Speaker-group online status = AND of every member's Online.
	for _, dvc := range devices {
		if dvc.DeviceFamily != amzconst.SpeakerGroupFamily {
			continue
		}
		all := true
		for memberSerial := range dvc.ClusterMembers {
			member, ok := devices[memberSerial]
			if !ok || !member.Online {
				all = false
				break
			}
		}
		dvc.Online = all
	}

	return nil
}

// parseSensorResponse walks the typed GraphQL response and returns a
// per-device sensor map keyed by canonical sensor name. Devices not
// present in either the endpoints map or the devices map are dropped.
//
// The typed client sends a single (non-batched) GraphQL request; the
// response is a single JSON object (not array). ArrayWrapper unwrapping
// is no longer needed.
func (s *SensorsService) parseSensorResponse(
	resp *alexagraphql.GetEndpointStateResponse,
	endpoints map[string]string,
	devices map[string]*Device,
) map[string]map[string]*DeviceSensor {
	out := map[string]map[string]*DeviceSensor{}
	if resp == nil || resp.ListEndpoints == nil {
		return out
	}
	for _, ep := range resp.ListEndpoints.Endpoints {
		if ep == nil {
			continue
		}
		serial, ok := endpoints[ep.EndpointId]
		if !ok {
			continue
		}
		dvc, ok := devices[serial]
		if !ok {
			continue
		}
		out[serial] = s.parseDeviceSensors(ep, dvc)
	}
	return out
}

// epEndpoint is a short local alias for the generated endpoint type.
type epEndpoint = alexagraphql.EndpointStateEndpoint

// epFeature is a short local alias for the generated feature type.
type epFeature = alexagraphql.EndpointStateFeature

// epProperty is a short local alias for the generated property interface.
type epProperty = alexagraphql.EndpointStateProperty

// parseDeviceSensors extracts per-sensor entries from one endpoint's
// features. Returns a possibly empty map.
func (s *SensorsService) parseDeviceSensors(
	endpoint *epEndpoint,
	dvc *Device,
) map[string]*DeviceSensor {
	out := map[string]*DeviceSensor{}
	for _, feature := range endpoint.Features {
		if feature == nil {
			continue
		}
		template, known := amzconst.Sensors[feature.Name]
		if !known {
			continue
		}
		for _, propPtr := range feature.Properties {
			if propPtr == nil {
				continue
			}
			sensor := s.buildSensor(template, *propPtr, feature, dvc)
			if sensor == nil {
				continue
			}
			out[sensor.Name] = sensor
		}
	}
	return out
}

// buildSensor converts one typed (feature, property) pair into a *DeviceSensor.
// Returns nil if the property carries an error type that upstream Python drops
// (NOT_FOUND), or if the concrete property type doesn't match the template.
func (s *SensorsService) buildSensor(
	template amzconst.SensorTemplate,
	prop epProperty,
	feature *epFeature,
	dvc *Device,
) *DeviceSensor {
	name := template.Name
	scale := template.Scale

	// Check for API-level error present on all property types via interface.
	apiErr := prop.GetError()
	hasError := apiErr != nil
	var errType, errMsg string
	if hasError {
		errType = apiErr.Type
		if apiErr.Message != nil {
			errMsg = *apiErr.Message
		}
	}

	if errType == "NOT_FOUND" {
		return nil
	}

	var value any = "n/a"

	if !hasError {
		// Extract value by type-switching on the concrete property type.
		// Each case mirrors what the old map[string]any path did using the
		// SensorTemplate.Key / Subkey / Scale fields. The old code also
		// filtered by prop["name"] == template.Name; that filter is now
		// implicit in the type switch — each feature carries one property
		// kind, and the switch's missing-case path returns nil.
		switch p := prop.(type) {
		case *alexagraphql.EndpointStatePropReachability:
			if p.ReachabilityStatusValue == nil {
				return nil
			}
			value = string(*p.ReachabilityStatusValue)

		case *alexagraphql.EndpointStatePropTemperature:
			if p.Value == nil {
				return nil
			}
			value = p.Value.Value
			scale = string(p.Value.Scale)

		case *alexagraphql.EndpointStatePropDetectionState:
			if p.DetectionStateValue == nil {
				return nil
			}
			value = string(*p.DetectionStateValue)

		case *alexagraphql.EndpointStatePropIlluminance:
			if p.IlluminanceValue == nil || p.IlluminanceValue.Value == nil {
				return nil
			}
			value = *p.IlluminanceValue.Value

		case *alexagraphql.EndpointStatePropRangeValue:
			if p.RangeValue == nil {
				return nil
			}
			value = p.RangeValue.Value

		case *alexagraphql.EndpointStatePropPower:
			if p.PowerStateValue == nil {
				return nil
			}
			value = string(*p.PowerStateValue)

		case *alexagraphql.EndpointStatePropBrightness:
			if p.BrightnessStateValue == nil {
				return nil
			}
			value = *p.BrightnessStateValue

		case *alexagraphql.EndpointStatePropColor:
			if p.ColorStateValue == nil {
				return nil
			}
			value = map[string]any{
				"hue":        p.ColorStateValue.Hue,
				"saturation": p.ColorStateValue.Saturation,
				"brightness": p.ColorStateValue.Brightness,
			}

		case *alexagraphql.EndpointStatePropColorTemperature:
			if p.ColorTemperatureInKelvinStateValue == nil {
				return nil
			}
			value = *p.ColorTemperatureInKelvinStateValue

		case *alexagraphql.EndpointStatePropLock:
			if p.LockState == nil {
				return nil
			}
			value = string(*p.LockState)

		case *alexagraphql.EndpointStatePropSetpoint:
			if p.Value == nil {
				return nil
			}
			value = p.Value.Value
			scale = string(p.Value.Scale)
			// Override the template Name for Setpoint so the result key
			// in Sensors is "targetSetpoint" not "thermostat".
			name = "targetSetpoint"

		case *alexagraphql.EndpointStatePropThermostatMode:
			if p.ThermostatModeValue == nil {
				return nil
			}
			value = string(*p.ThermostatModeValue)
			// Override the template Name so the result key is
			// "thermostatMode".
			name = "thermostatMode"

		case *alexagraphql.EndpointStatePropMode:
			// Generic Alexa.ModeController state. Distinct from
			// ThermostatMode — these are skill-defined string modes
			// (e.g. "Cycle.Auto", "WashCycle.Eco"). Instance-based
			// key rewrite happens below.
			if p.ModeValue == nil {
				return nil
			}
			value = p.ModeValue.Value

		case *alexagraphql.EndpointStatePropToggleState:
			// Generic Alexa.ToggleController state. "ON" / "OFF".
			// Multiple instances per device are possible (e.g. child
			// lock toggle + ionizer toggle); instance-based key
			// rewrite happens below.
			if p.ToggleStateValue == nil {
				return nil
			}
			value = string(*p.ToggleStateValue)

		case *alexagraphql.EndpointStatePropEnablementState:
			// EnablementState appears as a motion-sensor sub-property
			// (ENABLED / DISABLED / UNKNOWN). All motion-sensor
			// sub-properties share the "motionSensor" feature template
			// (Name = "detectionState"), so each case rewrites name
			// to disambiguate them in the final Sensors map.
			if p.EnablementStateValue == nil {
				return nil
			}
			value = string(*p.EnablementStateValue)
			name = "enablement"

		case *alexagraphql.EndpointStatePropDetectionSensitivity:
			if p.DetectionSensitivityValue == nil {
				return nil
			}
			value = string(*p.DetectionSensitivityValue)
			name = "detectionSensitivity"

		case *alexagraphql.EndpointStatePropDetectionRange:
			if p.DetectionRangeValue == nil {
				return nil
			}
			value = string(*p.DetectionRangeValue)
			name = "detectionRange"

		case *alexagraphql.EndpointStatePropDetectionTechnology:
			if p.DetectionTechnologyValue == nil {
				return nil
			}
			value = string(*p.DetectionTechnologyValue)
			name = "detectionTechnology"

		default:
			// Property type not among the handled set; skip.
			return nil
		}
	}

	// Range and Mode features both reuse the same wire-level typename
	// (RangeValue / a mode-property of varying shape) but Amazon
	// distinguishes them by feature.instance. Rewrite the sensor name
	// and scale based on instance, with these tiers:
	//   - AQM-specific instances (legacy) via amzconst.AQMRangeSensors.
	//   - Canonical Range instances (Apparatus.Humidity, …) via
	//     amzconst.RangeInstances.
	//   - Canonical Mode instances (Apparatus.FanSpeed, …) via
	//     amzconst.ModeInstances.
	//   - Anything else falls through to "range:<instance>" or
	//     "mode:<instance>" so callers see SOMETHING under a stable key.
	if feature != nil && feature.Instance != nil && *feature.Instance != "" {
		instance := *feature.Instance
		if template.Name == "rangeValue" {
			if dvc.DeviceType == amzconst.DeviceTypeAQM {
				aqm, ok := amzconst.AQMRangeSensors[instance]
				if !ok {
					return nil
				}
				name = aqm.Name
				scale = aqm.Scale
			} else if canonical, ok := amzconst.RangeInstances[instance]; ok {
				name = canonical.Name
				scale = canonical.Scale
			} else {
				name = "range:" + instance
			}
		}
		if feature.Name == "mode" {
			if canonical, ok := amzconst.ModeInstances[instance]; ok {
				name = canonical.Name
				scale = canonical.Scale
			} else {
				name = "mode:" + instance
			}
		}
		if feature.Name == "toggle" {
			// No canonical ToggleInstances table yet — skill-specific
			// instances vary widely (e.g. "ChildLock", "Ionizer",
			// "Swing"). Surface the raw instance so callers can route
			// on a stable key per device. The canonical table can be
			// added later as patterns emerge from HAR captures.
			name = "toggle:" + instance
		}
	}

	sensor := &DeviceSensor{
		Name:      name,
		Value:     value,
		Error:     hasError,
		ErrorType: errType,
		ErrorMsg:  errMsg,
		Scale:     scale,
	}

	// Surface the feature's action manifest + typed configuration on
	// every sensor produced from this feature.
	if feature != nil {
		if ops := extractFeatureOperations(feature); len(ops) > 0 {
			sensor.Operations = ops
		}
		if cfg := extractFeatureConfiguration(feature); len(cfg) > 0 {
			sensor.Configuration = cfg
		}
	}

	return sensor
}

// extractFeatureOperations pulls the operation names off a typed
// feature, in stable order. Empty when the feature has no operations.
func extractFeatureOperations(feature *epFeature) []string {
	if feature == nil {
		return nil
	}
	out := make([]string, 0, len(feature.Operations))
	for _, op := range feature.Operations {
		if op != nil && op.Name != "" {
			out = append(out, op.Name)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// extractFeatureConfiguration pulls the typed feature configuration into
// a map[string]any. Currently only ThermostatConfiguration is unpacked
// (its supportedModes are the action space for thermostat writes).
// Other configuration variants get a placeholder
// {"_type": "<TypeName>"} so callers can detect their presence.
func extractFeatureConfiguration(feature *epFeature) map[string]any {
	if feature == nil || feature.Configuration == nil {
		return nil
	}
	switch cfg := (*feature.Configuration).(type) {
	case *alexagraphql.EndpointStateConfigThermostat:
		// SupportedModes is []ThermostatModeValue (a typed enum slice);
		// convert to []string so callers don't need to know the generated
		// enum type.
		modes := make([]string, 0, len(cfg.SupportedModes))
		for _, m := range cfg.SupportedModes {
			modes = append(modes, string(m))
		}
		return map[string]any{
			"_type":          "ThermostatConfiguration",
			"supportedModes": modes,
		}
	case *alexagraphql.EndpointStateConfigMode:
		// ModeConfiguration carries the list of valid mode values for
		// this Mode-feature instance (e.g. Genesis.ColorPalette ->
		// {"Flare","Spectrum",…}). Callers use these to validate
		// setMode inputs and surface menus.
		out := map[string]any{"_type": "ModeConfiguration"}
		if cfg.FriendlyName != nil && cfg.FriendlyName.Value != nil {
			out["friendlyName"] = cfg.FriendlyName.Value.Text
		}
		opts := make([]map[string]any, 0, len(cfg.ModeOptions))
		for _, o := range cfg.ModeOptions {
			if o == nil {
				continue
			}
			entry := map[string]any{"value": o.Value}
			if o.ModeResources != nil && o.ModeResources.FriendlyName != nil &&
				o.ModeResources.FriendlyName.Value != nil {
				entry["friendlyName"] = o.ModeResources.FriendlyName.Value.Text
			}
			opts = append(opts, entry)
		}
		out["modeOptions"] = opts
		return out
	case *alexagraphql.EndpointStateConfigRange:
		// RangeConfiguration carries the supported numeric range
		// (min/max/precision), the unit string, and any named presets
		// (e.g. Fast=8.0). Callers use these to bound setRangeValue
		// inputs and surface presets in UIs.
		out := map[string]any{"_type": "RangeConfiguration"}
		if cfg.FriendlyName != nil && cfg.FriendlyName.Value != nil {
			out["friendlyName"] = cfg.FriendlyName.Value.Text
		}
		if cfg.SupportedRange != nil {
			out["supportedRange"] = map[string]any{
				"min":       cfg.SupportedRange.MinimumValue,
				"max":       cfg.SupportedRange.MaximumValue,
				"precision": cfg.SupportedRange.Precision,
			}
		}
		if cfg.UnitOfMeasure != nil && cfg.UnitOfMeasure.Value != nil {
			out["unitOfMeasure"] = cfg.UnitOfMeasure.Value.Text
		}
		presets := make([]map[string]any, 0, len(cfg.Presets))
		for _, p := range cfg.Presets {
			if p == nil {
				continue
			}
			entry := map[string]any{"value": p.RangeValue}
			if p.PresetResources.FriendlyName != nil &&
				p.PresetResources.FriendlyName.Value != nil {
				entry["friendlyName"] = p.PresetResources.FriendlyName.Value.Text
			}
			presets = append(presets, entry)
		}
		if len(presets) > 0 {
			out["presets"] = presets
		}
		return out
	default:
		typename := ""
		if tn := cfg.GetTypename(); tn != nil {
			typename = *tn
		}
		return map[string]any{
			"_type": typename,
		}
	}
}
