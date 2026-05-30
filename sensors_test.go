package amazondevices

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newSensorsTestClient builds a Client pointed at srv with a pre-installed
// Session so SensorsService can resolve its AccountCustomerID without going
// through Auth.Login.
//
// Uses the hostRewriter Transport pattern (see auth_login_test.go) so the
// production code keeps building real https://alexa.amazon.com URLs while
// every outbound request gets redirected to the httptest server.
func newSensorsTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	sess := &Session{
		AccountCustomerID: "amzn1.account.TEST",
		Site:              "https://www.amazon.com",
		Domain:            "com",
		DeviceInfo: DeviceInfo{
			DeviceSerialNumber: "TEST-SERIAL",
			DeviceType:         "TEST-TYPE",
		},
	}
	hc := &http.Client{Transport: &hostRewriter{base: srv.URL, inner: srv.Client().Transport}}
	c, err := NewClient(
		WithHTTPClient(hc),
		WithSite("https://www.amazon.com"),
		WithSession(sess),
	)
	require.NoError(t, err)
	return c
}

func TestSensorsService_Update_PostsCorrectEndpointIds(t *testing.T) {
	body, err := os.ReadFile("testdata/sensors/getEndpointState.json")
	require.NoError(t, err)

	var gotPath, gotMethod string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	c := newSensorsTestClient(t, srv)
	devices := map[string]*Device{
		"S-KITCHEN-001": {SerialNumber: "S-KITCHEN-001", DeviceType: "A18O6U1U9KIYDM",
			Sensors: map[string]*DeviceSensor{}},
	}
	endpoints := map[string]string{"ep-kitchen": "S-KITCHEN-001"}

	err = c.Sensors.Update(context.Background(), devices, endpoints, nil)
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(gotPath, "/nexus/v1/graphql"),
		"unexpected path: %s", gotPath)
	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "getEndpointState", gotBody["operationName"])
	vars, _ := gotBody["variables"].(map[string]any)
	require.NotNil(t, vars)
	epIDs, _ := vars["endpointIds"].([]any)
	require.Len(t, epIDs, 1)
	assert.Equal(t, "ep-kitchen", epIDs[0])
}

func TestSensorsService_Update_EmptyEndpointsSkipsHTTP(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	c := newSensorsTestClient(t, srv)
	err := c.Sensors.Update(context.Background(),
		map[string]*Device{}, map[string]string{}, nil)
	require.NoError(t, err)
	assert.False(t, called, "Update with empty endpoints should not call the GraphQL endpoint")
}

func TestSensorsService_Update_AppliesTemperatureAndOnline(t *testing.T) {
	body, err := os.ReadFile("testdata/sensors/getEndpointState.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	c := newSensorsTestClient(t, srv)
	kitchen := &Device{
		SerialNumber: "S-KITCHEN-001",
		DeviceType:   "A18O6U1U9KIYDM",
		Sensors:      map[string]*DeviceSensor{},
	}
	bedroom := &Device{
		SerialNumber: "S-BEDROOM-001",
		DeviceType:   "A18O6U1U9KIYDM",
		Sensors:      map[string]*DeviceSensor{},
	}
	devices := map[string]*Device{
		kitchen.SerialNumber: kitchen,
		bedroom.SerialNumber: bedroom,
	}
	endpoints := map[string]string{
		"ep-kitchen": kitchen.SerialNumber,
		"ep-bedroom": bedroom.SerialNumber,
	}
	require.NoError(t, c.Sensors.Update(context.Background(), devices, endpoints, nil))

	require.NotNil(t, kitchen.Sensors["reachability"])
	assert.Equal(t, "OK", kitchen.Sensors["reachability"].Value)
	assert.True(t, kitchen.Online)
	require.NotNil(t, kitchen.Sensors["temperature"])
	assert.Equal(t, 21.5, kitchen.Sensors["temperature"].Value)
	assert.Equal(t, "CELSIUS", kitchen.Sensors["temperature"].Scale)

	assert.False(t, bedroom.Online, "bedroom should be offline (reachability=UNAVAILABLE)")
}

func TestSensorsService_Update_ExpandsAQMRangeSubSensors(t *testing.T) {
	body, err := os.ReadFile("testdata/sensors/getEndpointState.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	c := newSensorsTestClient(t, srv)
	aqm := &Device{
		SerialNumber: "S-AQM-001",
		DeviceType:   "AEZME1X38KDRA",
		Sensors:      map[string]*DeviceSensor{},
	}
	devices := map[string]*Device{aqm.SerialNumber: aqm}
	endpoints := map[string]string{"ep-aqm-1": aqm.SerialNumber}

	require.NoError(t, c.Sensors.Update(context.Background(), devices, endpoints, nil))

	require.NotNil(t, aqm.Sensors["Humidity"], "Humidity (instance=4) should be present")
	assert.Equal(t, 47.0, aqm.Sensors["Humidity"].Value)
	assert.Equal(t, "%", aqm.Sensors["Humidity"].Scale)

	require.NotNil(t, aqm.Sensors["PM25"], "PM25 (instance=6) should be present")
	assert.Equal(t, 12.0, aqm.Sensors["PM25"].Value)
	assert.Equal(t, "MicroGramsPerCubicMeter", aqm.Sensors["PM25"].Scale)
}

func TestSensorsService_Update_OfflineWhenNoSensors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"listEndpoints":{"endpoints":[]}}}`))
	}))
	t.Cleanup(srv.Close)

	c := newSensorsTestClient(t, srv)
	dvc := &Device{
		SerialNumber: "S-KITCHEN-001",
		Online:       true,
		Sensors: map[string]*DeviceSensor{
			"temperature": {Name: "temperature", Value: 22.0},
		},
	}
	devices := map[string]*Device{dvc.SerialNumber: dvc}
	endpoints := map[string]string{"ep-kitchen": dvc.SerialNumber}

	require.NoError(t, c.Sensors.Update(context.Background(), devices, endpoints, nil))
	assert.False(t, dvc.Online, "device with no sensors in response should be marked offline")
	assert.True(t, dvc.Sensors["temperature"].Error, "stale sensor entries should be marked Error=true")
}

func TestSensorsService_Update_FeatureErrorFlagsSensor(t *testing.T) {
	body, err := os.ReadFile("testdata/sensors/getEndpointState.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	c := newSensorsTestClient(t, srv)
	dvc := &Device{
		SerialNumber: "S-ERRORED-001",
		Sensors:      map[string]*DeviceSensor{},
	}
	devices := map[string]*Device{dvc.SerialNumber: dvc}
	endpoints := map[string]string{"ep-aqm-error": dvc.SerialNumber}

	require.NoError(t, c.Sensors.Update(context.Background(), devices, endpoints, nil))
	require.NotNil(t, dvc.Sensors["reachability"])
	assert.True(t, dvc.Sensors["reachability"].Error)
	assert.Equal(t, "INTERNAL_ERROR", dvc.Sensors["reachability"].ErrorType)
	assert.False(t, dvc.Online)
}

func TestDevicesRefresh_RunsSensorEnrichment(t *testing.T) {
	devicesBody := []byte(`{
		"devices": [{
			"serialNumber": "S-KITCHEN-001",
			"deviceType": "A18O6U1U9KIYDM",
			"accountName": "Kitchen Echo",
			"deviceFamily": "KNIGHT",
			"deviceOwnerCustomerId": "amzn1.account.TEST",
			"online": true,
			"capabilities": ["AUDIO_PLAYER", "TIMERS_AND_ALARMS"],
			"clusterMembers": []
		}]
	}`)

	endpointsBody := []byte(`{
		"data": {
			"allDevices": {
				"endpoints": [{
					"endpointId": "ep-kitchen",
					"friendlyNameObject": {"value": {"text": "Kitchen Echo"}},
					"manufacturer": {"value": {"text": "Amazon"}},
					"serialNumber": {"value": {"text": "S-KITCHEN-001"}},
					"displayCategories": {
						"primary": {"value": "SPEAKER"},
						"all": [{"value": "SPEAKER"}]
					},
					"legacyIdentifiers": {
						"dmsIdentifier": {"deviceType": {"value": {"text": "A18O6U1U9KIYDM"}}}
					}
				}]
			}
		}
	}`)

	sensorsBody, err := os.ReadFile("testdata/sensors/getEndpointState.json")
	require.NoError(t, err)
	dndBody, err := os.ReadFile("testdata/dnd/status.json")
	require.NoError(t, err)
	notificationsBody, err := os.ReadFile("testdata/schedules/get_response.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/api/devices-v2/device"):
			_, _ = w.Write(devicesBody)
		case strings.HasSuffix(r.URL.Path, "/nexus/v1/graphql"):
			var payload any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			s, _ := json.Marshal(payload)
			if strings.Contains(string(s), "getEndpointState") {
				_, _ = w.Write(sensorsBody)
			} else {
				_, _ = w.Write(endpointsBody)
			}
		case strings.HasSuffix(r.URL.Path, "/api/dnd/device-status-list"):
			_, _ = w.Write(dndBody)
		case strings.HasSuffix(r.URL.Path, "/api/notifications"):
			_, _ = w.Write(notificationsBody)
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)

	c := newSensorsTestClient(t, srv)

	require.NoError(t, c.Devices.Refresh(context.Background()))

	dvc, err := c.Devices.Get(context.Background(), "S-KITCHEN-001")
	require.NoError(t, err)

	require.NotNil(t, dvc.Sensors["reachability"])
	assert.Equal(t, "OK", dvc.Sensors["reachability"].Value)
	require.NotNil(t, dvc.Sensors["temperature"])
	assert.Equal(t, 21.5, dvc.Sensors["temperature"].Value)
	require.NotNil(t, dvc.Sensors["dnd"], "DnD entry should be plumbed in by sensorsHook")
	assert.Equal(t, true, dvc.Sensors["dnd"].Value)
}

func TestDevicesRefresh_WithoutSensors_SkipsExtraCalls(t *testing.T) {
	var graphqlCount, dndCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/api/devices-v2/device"):
			_, _ = w.Write([]byte(`{"devices": []}`))
		case strings.HasSuffix(r.URL.Path, "/nexus/v1/graphql"):
			graphqlCount++
			var payload any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			s, _ := json.Marshal(payload)
			if strings.Contains(string(s), "getEndpointState") {
				t.Errorf("getEndpointState should NOT be called with WithoutSensors()")
			}
			_, _ = w.Write([]byte(`{"data":{"allDevices":{"endpoints":[]}}}`))
		case strings.HasSuffix(r.URL.Path, "/api/dnd/device-status-list"):
			dndCount++
			_, _ = w.Write([]byte(`{"doNotDisturbDeviceStatusList":[]}`))
		}
	}))
	t.Cleanup(srv.Close)

	c := newSensorsTestClient(t, srv)
	require.NoError(t, c.Devices.Refresh(context.Background(), WithoutSensors()))
	assert.Equal(t, 1, graphqlCount, "only the device-base GraphQL call should fire")
	assert.Equal(t, 0, dndCount, "DnD should not be called when sensors are skipped")
}

func TestSensorsService_Update_AppliesSmartHomePropertyTypes(t *testing.T) {
	body, err := os.ReadFile("testdata/sensors/getEndpointState.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	c := newSensorsTestClient(t, srv)
	devices := map[string]*Device{
		"S-LIGHT-001":      {SerialNumber: "S-LIGHT-001", Sensors: map[string]*DeviceSensor{}},
		"S-THERMOSTAT-001": {SerialNumber: "S-THERMOSTAT-001", Sensors: map[string]*DeviceSensor{}},
		"S-LOCK-001":       {SerialNumber: "S-LOCK-001", Sensors: map[string]*DeviceSensor{}},
	}
	endpoints := map[string]string{
		"ep-light-001":      "S-LIGHT-001",
		"ep-thermostat-001": "S-THERMOSTAT-001",
		"ep-lock-001":       "S-LOCK-001",
	}

	require.NoError(t, c.Sensors.Update(context.Background(), devices, endpoints, nil))

	// Light: powerState + brightness + color.
	light := devices["S-LIGHT-001"]
	require.NotNil(t, light.Sensors["powerState"])
	assert.Equal(t, "ON", light.Sensors["powerState"].Value)
	require.NotNil(t, light.Sensors["brightness"])
	assert.EqualValues(t, 75, light.Sensors["brightness"].Value)
	require.NotNil(t, light.Sensors["color"])
	color, ok := light.Sensors["color"].Value.(map[string]any)
	require.True(t, ok, "color value should be a map")
	assert.InDelta(t, 120.0, color["hue"], 0.001)
	assert.InDelta(t, 0.5, color["saturation"], 0.001)
	assert.InDelta(t, 0.8, color["brightness"], 0.001)

	// Operations surfaced.
	assert.Equal(t, []string{"turnOn", "turnOff"}, light.Sensors["powerState"].Operations)
	assert.Equal(t, []string{"setBrightness", "adjustBrightness"}, light.Sensors["brightness"].Operations)

	// Thermostat: mode + setpoint.
	thermo := devices["S-THERMOSTAT-001"]
	require.NotNil(t, thermo.Sensors["thermostatMode"])
	assert.Equal(t, "COOL", thermo.Sensors["thermostatMode"].Value)
	require.NotNil(t, thermo.Sensors["targetSetpoint"])
	assert.EqualValues(t, 21.5, thermo.Sensors["targetSetpoint"].Value)
	assert.Equal(t, "CELSIUS", thermo.Sensors["targetSetpoint"].Scale)

	// Configuration surfaced on thermostat sensors.
	cfg := thermo.Sensors["thermostatMode"].Configuration
	require.NotNil(t, cfg, "thermostat sensor should carry its Configuration")
	assert.Equal(t, "ThermostatConfiguration", cfg["_type"])
	modes, ok := cfg["supportedModes"].([]string)
	require.True(t, ok, "supportedModes should be []string")
	assert.ElementsMatch(t, []string{"COOL", "HEAT", "AUTO"}, modes)

	// Lock: state.
	lock := devices["S-LOCK-001"]
	require.NotNil(t, lock.Sensors["lockState"])
	assert.Equal(t, "LOCKED", lock.Sensors["lockState"].Value)
	assert.Equal(t, []string{"lockDoor", "unlockDoor"}, lock.Sensors["lockState"].Operations)
}

func TestSensorsService_Update_CanonicalRangeInstanceHumidity(t *testing.T) {
	body, err := os.ReadFile("testdata/sensors/getEndpointState.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	c := newSensorsTestClient(t, srv)
	devices := map[string]*Device{
		"S-AC-001": {SerialNumber: "S-AC-001", DeviceType: "NOT-AN-AQM",
			Sensors: map[string]*DeviceSensor{}},
	}
	endpoints := map[string]string{"ep-ac-001": "S-AC-001"}

	require.NoError(t, c.Sensors.Update(context.Background(), devices, endpoints, nil))

	ac := devices["S-AC-001"]
	require.NotNil(t, ac.Sensors["humidity"], "Apparatus.Humidity should map to canonical 'humidity'")
	assert.EqualValues(t, 47.5, ac.Sensors["humidity"].Value)
	assert.Equal(t, "%", ac.Sensors["humidity"].Scale)
}

func TestSensorsService_Update_UnknownRangeInstanceFallsThrough(t *testing.T) {
	body, err := os.ReadFile("testdata/sensors/getEndpointState.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	c := newSensorsTestClient(t, srv)
	devices := map[string]*Device{
		"S-WEIRD-001": {SerialNumber: "S-WEIRD-001", DeviceType: "NOT-AN-AQM",
			Sensors: map[string]*DeviceSensor{}},
	}
	endpoints := map[string]string{"ep-weird-001": "S-WEIRD-001"}

	require.NoError(t, c.Sensors.Update(context.Background(), devices, endpoints, nil))

	weird := devices["S-WEIRD-001"]
	require.NotNil(t, weird.Sensors["range:Apparatus.WhateverNew"],
		"unknown range instance should fall through to range:<instance> key")
	assert.EqualValues(t, 99.0, weird.Sensors["range:Apparatus.WhateverNew"].Value)
}

// TestSensorsService_Update_ModePropertyParsed verifies that a Mode
// property (generic Alexa.ModeController state, distinct from
// ThermostatMode) is decoded and surfaced under a stable sensor key
// derived from feature.instance. Sets up an inline fixture because
// the shared getEndpointState.json doesn't carry a generic-Mode
// example yet.
func TestSensorsService_Update_ModePropertyParsed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
		  "data": {"listEndpoints": {"endpoints": [{
		    "endpointId": "ep-fan-001",
		    "friendlyNameObject": {"value": {"text": "Bedroom Fan"}},
		    "features": [
		      {
		        "name": "mode",
		        "instance": "Apparatus.FanSpeed",
		        "operations": [],
		        "configuration": null,
		        "properties": [{
		          "__typename": "Mode",
		          "name": "mode",
		          "type": "PRIMARY",
		          "accuracy": "EXACT",
		          "error": null,
		          "modeValue": {"value": "Speed.High"}
		        }]
		      },
		      {
		        "name": "mode",
		        "instance": "Skill.CycleProgram",
		        "operations": [],
		        "configuration": null,
		        "properties": [{
		          "__typename": "Mode",
		          "name": "mode",
		          "type": "PRIMARY",
		          "accuracy": "EXACT",
		          "error": null,
		          "modeValue": {"value": "Cycle.Eco"}
		        }]
		      }
		    ]
		  }]}}
		}`))
	}))
	t.Cleanup(srv.Close)

	c := newSensorsTestClient(t, srv)
	devices := map[string]*Device{
		"S-FAN-001": {SerialNumber: "S-FAN-001", Sensors: map[string]*DeviceSensor{}},
	}
	endpoints := map[string]string{"ep-fan-001": "S-FAN-001"}

	require.NoError(t, c.Sensors.Update(context.Background(), devices, endpoints, nil))

	fan := devices["S-FAN-001"]
	// Apparatus.FanSpeed has a canonical mapping → "fanSpeed".
	require.NotNil(t, fan.Sensors["fanSpeed"], "Apparatus.FanSpeed should map to canonical 'fanSpeed' key")
	assert.Equal(t, "Speed.High", fan.Sensors["fanSpeed"].Value)
	// Unknown instance falls through to "mode:<instance>".
	require.NotNil(t, fan.Sensors["mode:Skill.CycleProgram"],
		"unknown mode instance should fall through to mode:<instance> key")
	assert.Equal(t, "Cycle.Eco", fan.Sensors["mode:Skill.CycleProgram"].Value)
}

// TestSensorsService_Update_ToggleStatePropertyParsed verifies that a
// ToggleState property (generic Alexa.ToggleController state) is
// decoded and surfaced under "toggle:<instance>". No canonical
// ToggleInstances table exists yet, so every toggle uses the
// instance-suffixed key.
func TestSensorsService_Update_ToggleStatePropertyParsed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
		  "data": {"listEndpoints": {"endpoints": [{
		    "endpointId": "ep-purifier-001",
		    "friendlyNameObject": {"value": {"text": "Hall Purifier"}},
		    "features": [
		      {
		        "name": "toggle",
		        "instance": "Skill.Ionizer",
		        "operations": [],
		        "configuration": null,
		        "properties": [{
		          "__typename": "ToggleState",
		          "name": "toggleState",
		          "type": "PRIMARY",
		          "accuracy": "EXACT",
		          "error": null,
		          "toggleStateValue": "ON"
		        }]
		      },
		      {
		        "name": "toggle",
		        "instance": "Skill.ChildLock",
		        "operations": [],
		        "configuration": null,
		        "properties": [{
		          "__typename": "ToggleState",
		          "name": "toggleState",
		          "type": "PRIMARY",
		          "accuracy": "EXACT",
		          "error": null,
		          "toggleStateValue": "OFF"
		        }]
		      }
		    ]
		  }]}}
		}`))
	}))
	t.Cleanup(srv.Close)

	c := newSensorsTestClient(t, srv)
	devices := map[string]*Device{
		"S-PURIFIER-001": {SerialNumber: "S-PURIFIER-001", Sensors: map[string]*DeviceSensor{}},
	}
	endpoints := map[string]string{"ep-purifier-001": "S-PURIFIER-001"}

	require.NoError(t, c.Sensors.Update(context.Background(), devices, endpoints, nil))

	purifier := devices["S-PURIFIER-001"]
	require.NotNil(t, purifier.Sensors["toggle:Skill.Ionizer"])
	assert.Equal(t, "ON", purifier.Sensors["toggle:Skill.Ionizer"].Value)
	require.NotNil(t, purifier.Sensors["toggle:Skill.ChildLock"])
	assert.Equal(t, "OFF", purifier.Sensors["toggle:Skill.ChildLock"].Value)
}

// TestSensorsService_Update_MotionSensorSubProperties verifies that
// the four motion-sensor sub-properties (EnablementState,
// DetectionSensitivity, DetectionRange, DetectionTechnology) each
// surface under their own Sensors key. Before this fix they all came
// back as UnknownFeatureProperty because the GraphQL fragment didn't
// request them, so 80% of an Echo Show / Echo Studio's motion-sensor
// configuration was silently dropped.
func TestSensorsService_Update_MotionSensorSubProperties(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
		  "data": {"listEndpoints": {"endpoints": [{
		    "endpointId": "ep-echo-show",
		    "friendlyNameObject": {"value": {"text": "Bedroom Echo Show"}},
		    "features": [{
		      "name": "motionSensor",
		      "instance": null,
		      "operations": [],
		      "configuration": null,
		      "properties": [
		        {"__typename": "DetectionState", "name": "detectionState", "type": "PRIMARY", "accuracy": "HIGH", "error": null, "detectionStateValue": "DETECTED"},
		        {"__typename": "EnablementState", "name": "enablement", "type": null, "accuracy": "HIGH", "error": null, "enablementStateValue": "ENABLED"},
		        {"__typename": "DetectionSensitivity", "name": "detectionSensitivity", "type": null, "accuracy": "HIGH", "error": null, "detectionSensitivityValue": "MEDIUM"},
		        {"__typename": "DetectionRange", "name": "detectionRange", "type": null, "accuracy": "HIGH", "error": null, "detectionRangeValue": "HIGH"},
		        {"__typename": "DetectionTechnology", "name": "detectionTechnology", "type": null, "accuracy": "HIGH", "error": null, "detectionTechnologyValue": "ULTRASONIC_ENHANCED_WITH_WIFI_CSI"}
		      ]
		    }]
		  }]}}
		}`))
	}))
	t.Cleanup(srv.Close)

	c := newSensorsTestClient(t, srv)
	devices := map[string]*Device{
		"S-SHOW-001": {SerialNumber: "S-SHOW-001", Sensors: map[string]*DeviceSensor{}},
	}
	endpoints := map[string]string{"ep-echo-show": "S-SHOW-001"}

	require.NoError(t, c.Sensors.Update(context.Background(), devices, endpoints, nil))

	show := devices["S-SHOW-001"]
	require.NotNil(t, show.Sensors["detectionState"])
	assert.Equal(t, "DETECTED", show.Sensors["detectionState"].Value)
	require.NotNil(t, show.Sensors["enablement"])
	assert.Equal(t, "ENABLED", show.Sensors["enablement"].Value)
	require.NotNil(t, show.Sensors["detectionSensitivity"])
	assert.Equal(t, "MEDIUM", show.Sensors["detectionSensitivity"].Value)
	require.NotNil(t, show.Sensors["detectionRange"])
	assert.Equal(t, "HIGH", show.Sensors["detectionRange"].Value)
	require.NotNil(t, show.Sensors["detectionTechnology"])
	assert.Equal(t, "ULTRASONIC_ENHANCED_WITH_WIFI_CSI", show.Sensors["detectionTechnology"].Value)
}

// TestSensorsService_Update_ModeConfigurationParsed verifies that
// ModeConfiguration on a Mode feature is decoded into the
// Sensors[...].Configuration map with the full modeOptions list +
// friendly names. Without this, callers picking valid setMode
// values would have to trial-and-error against the device.
func TestSensorsService_Update_ModeConfigurationParsed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
		  "data": {"listEndpoints": {"endpoints": [{
		    "endpointId": "ep-purifier-001",
		    "friendlyNameObject": {"value": {"text": "Hall Purifier"}},
		    "features": [{
		      "name": "mode",
		      "instance": "Pure.FanSpeed",
		      "operations": [],
		      "configuration": {
		        "__typename": "ModeConfiguration",
		        "friendlyName": {"value": {"text": "Fan Speed"}},
		        "modeOptions": [
		          {"value": "FanSpeed.Low", "modeResources": {"friendlyName": {"value": {"text": "Low"}}}},
		          {"value": "FanSpeed.High", "modeResources": {"friendlyName": {"value": {"text": "High"}}}}
		        ]
		      },
		      "properties": [{
		        "__typename": "Mode", "name": "mode", "type": "PRIMARY", "accuracy": "EXACT", "error": null,
		        "modeValue": {"value": "FanSpeed.Low"}
		      }]
		    }]
		  }]}}
		}`))
	}))
	t.Cleanup(srv.Close)

	c := newSensorsTestClient(t, srv)
	devices := map[string]*Device{
		"S-PUR-001": {SerialNumber: "S-PUR-001", Sensors: map[string]*DeviceSensor{}},
	}
	endpoints := map[string]string{"ep-purifier-001": "S-PUR-001"}
	require.NoError(t, c.Sensors.Update(context.Background(), devices, endpoints, nil))

	s := devices["S-PUR-001"].Sensors["mode:Pure.FanSpeed"]
	require.NotNil(t, s)
	require.NotNil(t, s.Configuration)
	assert.Equal(t, "ModeConfiguration", s.Configuration["_type"])
	assert.Equal(t, "Fan Speed", s.Configuration["friendlyName"])
	opts, ok := s.Configuration["modeOptions"].([]map[string]any)
	require.True(t, ok, "modeOptions should be a []map[string]any")
	require.Len(t, opts, 2)
	assert.Equal(t, "FanSpeed.Low", opts[0]["value"])
	assert.Equal(t, "Low", opts[0]["friendlyName"])
	assert.Equal(t, "FanSpeed.High", opts[1]["value"])
	assert.Equal(t, "High", opts[1]["friendlyName"])
}

// TestSensorsService_Update_RangeConfigurationParsed verifies the
// RangeConfiguration variant: min/max/precision bounds, unit, and
// any named presets surface under Configuration. Callers use these
// to validate setRangeValue inputs and offer presets in UIs.
func TestSensorsService_Update_RangeConfigurationParsed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
		  "data": {"listEndpoints": {"endpoints": [{
		    "endpointId": "ep-fan-001",
		    "friendlyNameObject": {"value": {"text": "Ceiling Fan"}},
		    "features": [{
		      "name": "range",
		      "instance": "Skill.FanSpeed",
		      "operations": [],
		      "configuration": {
		        "__typename": "RangeConfiguration",
		        "friendlyName": {"value": {"text": "Fan Speed"}},
		        "supportedRange": {"minimumValue": 0, "maximumValue": 10, "precision": 1},
		        "unitOfMeasure": {"value": {"text": "Percent"}},
		        "presets": [
		          {"rangeValue": 0, "presetResources": {"friendlyName": {"value": {"text": "Off"}}}},
		          {"rangeValue": 5, "presetResources": {"friendlyName": {"value": {"text": "Medium"}}}}
		        ]
		      },
		      "properties": [{
		        "__typename": "RangeValue", "name": "rangeValue", "type": "PRIMARY", "accuracy": "EXACT", "error": null,
		        "rangeValue": {"value": 5}
		      }]
		    }]
		  }]}}
		}`))
	}))
	t.Cleanup(srv.Close)

	c := newSensorsTestClient(t, srv)
	devices := map[string]*Device{
		"S-FAN-002": {SerialNumber: "S-FAN-002", Sensors: map[string]*DeviceSensor{}},
	}
	endpoints := map[string]string{"ep-fan-001": "S-FAN-002"}
	require.NoError(t, c.Sensors.Update(context.Background(), devices, endpoints, nil))

	s := devices["S-FAN-002"].Sensors["range:Skill.FanSpeed"]
	require.NotNil(t, s)
	require.NotNil(t, s.Configuration)
	assert.Equal(t, "RangeConfiguration", s.Configuration["_type"])
	assert.Equal(t, "Fan Speed", s.Configuration["friendlyName"])
	assert.Equal(t, "Percent", s.Configuration["unitOfMeasure"])
	bounds, ok := s.Configuration["supportedRange"].(map[string]any)
	require.True(t, ok)
	assert.EqualValues(t, 0.0, bounds["min"])
	assert.EqualValues(t, 10.0, bounds["max"])
	assert.EqualValues(t, 1.0, bounds["precision"])
	presets, ok := s.Configuration["presets"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, presets, 2)
	assert.EqualValues(t, 0.0, presets[0]["value"])
	assert.Equal(t, "Off", presets[0]["friendlyName"])
	assert.EqualValues(t, 5.0, presets[1]["value"])
	assert.Equal(t, "Medium", presets[1]["friendlyName"])
}
