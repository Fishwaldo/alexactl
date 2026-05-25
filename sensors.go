package amazondevices

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/Fishwaldo/goamazondevices/internal/amzconst"
	"github.com/Fishwaldo/goamazondevices/internal/httpwrapper"
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

// fetchEndpointStates POSTs the getEndpointState GraphQL operation for
// every endpointID in endpoints and returns the wrapper-decoded response.
// Returns (nil, nil) if endpoints is empty so callers can short-circuit
// without an HTTP request.
func (s *SensorsService) fetchEndpointStates(
	ctx context.Context,
	endpoints map[string]string,
) (map[string]any, error) {
	if len(endpoints) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(endpoints))
	for id := range endpoints {
		ids = append(ids, id)
	}

	payload := []map[string]any{
		{
			"operationName": "getEndpointState",
			"variables":     map[string]any{"endpointIds": ids},
			"query":         amzconst.QuerySensorState,
		},
	}

	url := fmt.Sprintf("https://alexa.amazon.%s%s",
		s.wrapper.Domain(), amzconst.URINexusGraphQL)

	resp, err := s.wrapper.Do(ctx, httpwrapper.Request{
		Method:       http.MethodPost,
		URL:          url,
		JSONBody:     payload,
		ExtraHeaders: http.Header{"User-Agent": []string{amzconst.UserAgentAmazon}},
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCannotConnect, err)
	}
	if resp.StatusCode >= 400 {
		body := resp.Body
		defer body.Close()
		return nil, &APIError{
			Category:   ErrCannotRetrieveData,
			Method:     http.MethodPost,
			StatusCode: resp.StatusCode,
			URL:        url,
		}
	}
	data, err := httpwrapper.DecodeJSON(resp)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCannotRetrieveData, err)
	}
	return data, nil
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
//   - If notifications is non-nil, copies each device's notification map
//     onto device.Notifications. (Phase 2a passes nil — sub-plan 2b wires
//     this.)
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
	notifications map[string]map[string]*Schedule,
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

		if notifications != nil {
			dvc.Notifications = map[string]*Schedule{}
			if n, ok := notifications[dvc.SerialNumber]; ok {
				for _, allow := range allowedNotificationTypes(dvc) {
					if sched, present := n[allow]; present {
						dvc.Notifications[allow] = sched
					}
				}
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

// parseSensorResponse walks the GraphQL response and returns a
// per-device sensor map keyed by canonical sensor name. Devices not
// present in either the endpoints map or the devices map are dropped.
func (s *SensorsService) parseSensorResponse(
	resp map[string]any,
	endpoints map[string]string,
	devices map[string]*Device,
) map[string]map[string]*DeviceSensor {
	out := map[string]map[string]*DeviceSensor{}
	if resp == nil {
		return out
	}
	dataNode, _ := resp["data"].(map[string]any)
	if dataNode == nil {
		return out
	}
	listEndpoints, _ := dataNode["listEndpoints"].(map[string]any)
	if listEndpoints == nil {
		return out
	}
	endpointsArr, _ := listEndpoints["endpoints"].([]any)

	for _, item := range endpointsArr {
		endpoint, _ := item.(map[string]any)
		if endpoint == nil {
			continue
		}
		endpointID, _ := endpoint["endpointId"].(string)
		serial, ok := endpoints[endpointID]
		if !ok {
			continue
		}
		dvc, ok := devices[serial]
		if !ok {
			continue
		}
		out[serial] = s.parseDeviceSensors(endpoint, dvc)
	}
	return out
}

// parseDeviceSensors extracts per-sensor entries from one endpoint's
// features. Returns a possibly empty map.
func (s *SensorsService) parseDeviceSensors(
	endpoint map[string]any,
	dvc *Device,
) map[string]*DeviceSensor {
	out := map[string]*DeviceSensor{}
	features, _ := endpoint["features"].([]any)
	for _, fi := range features {
		feature, _ := fi.(map[string]any)
		if feature == nil {
			continue
		}
		featureName, _ := feature["name"].(string)
		template, known := amzconst.Sensors[featureName]
		if !known {
			continue
		}
		props, _ := feature["properties"].([]any)
		for _, pi := range props {
			prop, _ := pi.(map[string]any)
			if prop == nil {
				continue
			}
			propName, _ := prop["name"].(string)
			if propName != template.Name {
				continue
			}
			sensor := s.buildSensor(template, prop, feature, dvc)
			if sensor == nil {
				continue
			}
			out[sensor.Name] = sensor
		}
	}
	return out
}

// buildSensor converts one (feature, property) pair into a *DeviceSensor.
// Returns nil if the property is malformed or carries an error type that
// upstream Python drops (NOT_FOUND).
func (s *SensorsService) buildSensor(
	template amzconst.SensorTemplate,
	prop map[string]any,
	feature map[string]any,
	dvc *Device,
) *DeviceSensor {
	name := template.Name
	scale := template.Scale
	var value any = "n/a"

	apiError, _ := prop["error"].(map[string]any)
	hasError := len(apiError) > 0
	errType, _ := apiError["type"].(string)
	errMsg, _ := apiError["message"].(string)

	if errType == "NOT_FOUND" {
		return nil
	}

	if !hasError {
		raw, present := prop[template.Key]
		if !present {
			return nil
		}
		switch v := raw.(type) {
		case map[string]any:
			if template.Subkey == "" {
				value = v
			} else if sub, ok := v[template.Subkey]; ok {
				value = sub
			} else {
				return nil
			}
			if template.Scale != "" {
				if sc, ok := v[template.Scale].(string); ok {
					scale = sc
				} else {
					scale = ""
				}
			}
		default:
			if template.Subkey != "" {
				return nil
			}
			value = v
			scale = ""
		}
	}

	// AQM range expansion: rewrite the sensor name + scale based on the
	// feature instance, mirroring upstream `aqm_sensor` lookup.
	if dvc.DeviceType == amzconst.DeviceTypeAQM && template.Name == "rangeValue" {
		instance, _ := feature["instance"].(string)
		aqm, ok := amzconst.AQMRangeSensors[instance]
		if !ok {
			return nil
		}
		name = aqm.Name
		scale = aqm.Scale
	}

	return &DeviceSensor{
		Name:      name,
		Value:     value,
		Error:     hasError,
		ErrorType: errType,
		ErrorMsg:  errMsg,
		Scale:     scale,
	}
}

// allowedNotificationTypes returns the notification types the device's
// capabilities allow us to surface. Mirrors upstream Python's
// implementation/sensor.py (capability, notification_type) filter:
//
//	REMINDERS         -> Reminder
//	TIMERS_AND_ALARMS -> Alarm, Timer
//
// Devices without the capability silently get no notifications of the
// corresponding type. Devices with no capabilities (e.g., AQMs surfaced
// by Phase-1 enrichment) get nothing.
func allowedNotificationTypes(dvc *Device) []string {
	var out []string
	for _, c := range dvc.Capabilities {
		switch c {
		case amzconst.CapabilityReminders:
			out = append(out, amzconst.NotificationReminder)
		case amzconst.CapabilityTimersAndAlarms:
			out = append(out, amzconst.NotificationAlarm, amzconst.NotificationTimer)
		}
	}
	return out
}
