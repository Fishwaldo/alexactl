// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package alexactl

import (
	"github.com/Fishwaldo/alexactl/internal/alexagraphql"
	"github.com/Fishwaldo/alexactl/internal/amzconst"
)

// parseAllFeaturesEndpoint converts the deep endpointAllFeatures
// response into a Device.Sensors-shaped map. Mirrors the structure of
// sensors.go:parseDeviceSensors + buildSensor, but type-switches on
// the deep-query property types (which genqlient generates as a
// separate polymorphic interface from the getEndpointState response).
//
// The duplication is intentional: the alternative — sharing a single
// polymorphic interface across both queries — requires either a
// genqlient bind directive (which doesn't fully work for inline
// fragments) or a runtime adapter type. The ~120-line duplication
// here is straightforward to read and has its behavior pinned by
// inspect_parser_test.go using the same Sideroom Airconditioner
// fixture.
//
// Returns a non-nil but possibly empty map.
func parseAllFeaturesEndpoint(
	ep *alexagraphql.EndpointAllFeaturesEndpoint,
	dvc *Device,
) map[string]*DeviceSensor {
	out := map[string]*DeviceSensor{}
	if ep == nil {
		return out
	}
	for _, feature := range ep.Features {
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
			sensor := buildAllFeaturesSensor(template, *propPtr, feature, dvc)
			if sensor == nil {
				continue
			}
			out[sensor.Name] = sensor
		}
	}
	return out
}

// buildAllFeaturesSensor is the deep-query equivalent of
// sensors.go:buildSensor. Same shape, different concrete property
// types in the switch arms.
func buildAllFeaturesSensor(
	template amzconst.SensorTemplate,
	prop alexagraphql.EndpointAllFeaturesProperty,
	feature *alexagraphql.EndpointAllFeaturesEndpointFeaturesFeature,
	dvc *Device,
) *DeviceSensor {
	name := template.Name
	scale := template.Scale

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
		switch p := prop.(type) {
		case *alexagraphql.EndpointAllFeaturesPropReachability:
			if p.ReachabilityStatusValue == nil {
				return nil
			}
			value = string(*p.ReachabilityStatusValue)
		case *alexagraphql.EndpointAllFeaturesPropTemperatureSensor:
			if p.Value == nil {
				return nil
			}
			value = p.Value.Value
			scale = string(p.Value.Scale)
		case *alexagraphql.EndpointAllFeaturesPropDetectionState:
			if p.DetectionStateValue == nil {
				return nil
			}
			value = string(*p.DetectionStateValue)
		case *alexagraphql.EndpointAllFeaturesPropIlluminance:
			if p.IlluminanceValue == nil || p.IlluminanceValue.Value == nil {
				return nil
			}
			value = *p.IlluminanceValue.Value
		case *alexagraphql.EndpointAllFeaturesPropRangeValue:
			if p.RangeValue == nil {
				return nil
			}
			value = p.RangeValue.Value
		case *alexagraphql.EndpointAllFeaturesPropPower:
			if p.PowerStateValue == nil {
				return nil
			}
			value = string(*p.PowerStateValue)
		case *alexagraphql.EndpointAllFeaturesPropBrightness:
			if p.BrightnessStateValue == nil {
				return nil
			}
			value = *p.BrightnessStateValue
		case *alexagraphql.EndpointAllFeaturesPropColor:
			if p.ColorStateValue == nil {
				return nil
			}
			value = map[string]any{
				"hue":        p.ColorStateValue.Hue,
				"saturation": p.ColorStateValue.Saturation,
				"brightness": p.ColorStateValue.Brightness,
			}
		case *alexagraphql.EndpointAllFeaturesPropColorTemperature:
			if p.ColorTemperatureInKelvinStateValue == nil {
				return nil
			}
			value = *p.ColorTemperatureInKelvinStateValue
		case *alexagraphql.EndpointAllFeaturesPropLock:
			if p.LockState == nil {
				return nil
			}
			value = string(*p.LockState)
		case *alexagraphql.EndpointAllFeaturesPropSetpoint:
			if p.Value == nil {
				return nil
			}
			value = p.Value.Value
			scale = string(p.Value.Scale)
			name = "targetSetpoint"
		case *alexagraphql.EndpointAllFeaturesPropThermostatMode:
			if p.ThermostatModeValue == nil {
				return nil
			}
			value = string(*p.ThermostatModeValue)
			name = "thermostatMode"
		case *alexagraphql.EndpointAllFeaturesPropMode:
			if p.ModeValue == nil {
				return nil
			}
			value = p.ModeValue.Value
		case *alexagraphql.EndpointAllFeaturesPropToggleState:
			if p.ToggleStateValue == nil {
				return nil
			}
			value = string(*p.ToggleStateValue)
		case *alexagraphql.EndpointAllFeaturesPropEnablementState:
			if p.EnablementStateValue == nil {
				return nil
			}
			value = string(*p.EnablementStateValue)
			name = "enablement"
		case *alexagraphql.EndpointAllFeaturesPropDetectionSensitivity:
			if p.DetectionSensitivityValue == nil {
				return nil
			}
			value = string(*p.DetectionSensitivityValue)
			name = "detectionSensitivity"
		case *alexagraphql.EndpointAllFeaturesPropDetectionRange:
			if p.DetectionRangeValue == nil {
				return nil
			}
			value = string(*p.DetectionRangeValue)
			name = "detectionRange"
		case *alexagraphql.EndpointAllFeaturesPropDetectionTechnology:
			if p.DetectionTechnologyValue == nil {
				return nil
			}
			value = string(*p.DetectionTechnologyValue)
			name = "detectionTechnology"
		default:
			return nil
		}
	}

	// Range/Mode instance rewrite — same three-tier logic as
	// sensors.go:buildSensor. Refactor target if a third caller ever
	// needs it.
	if feature.Instance != nil && *feature.Instance != "" {
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
	if ops := extractAllFeaturesOperations(feature); len(ops) > 0 {
		sensor.Operations = ops
	}
	if cfg := extractAllFeaturesConfiguration(feature); len(cfg) > 0 {
		sensor.Configuration = cfg
	}
	return sensor
}

// extractAllFeaturesOperations is the deep-query counterpart of
// sensors.go:extractFeatureOperations.
func extractAllFeaturesOperations(feature *alexagraphql.EndpointAllFeaturesEndpointFeaturesFeature) []string {
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

// extractAllFeaturesConfiguration is the deep-query counterpart of
// sensors.go:extractFeatureConfiguration. The generated config
// interface types differ between the two queries; the public shape
// (map[string]any with "_type" and variant-specific keys) is identical.
func extractAllFeaturesConfiguration(feature *alexagraphql.EndpointAllFeaturesEndpointFeaturesFeature) map[string]any {
	if feature == nil || feature.Configuration == nil {
		return nil
	}
	switch cfg := (*feature.Configuration).(type) {
	case *alexagraphql.EndpointAllFeaturesConfigThermostat:
		modes := make([]string, 0, len(cfg.SupportedModes))
		for _, m := range cfg.SupportedModes {
			modes = append(modes, string(m))
		}
		return map[string]any{
			"_type":          "ThermostatConfiguration",
			"supportedModes": modes,
		}
	case *alexagraphql.EndpointAllFeaturesConfigMode:
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
	case *alexagraphql.EndpointAllFeaturesConfigRange:
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
		return map[string]any{"_type": typename}
	}
}
