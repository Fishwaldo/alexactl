package amazondevices

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

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
	var gotBody []map[string]any
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

	err = c.Sensors.Update(context.Background(), devices, endpoints, nil, nil)
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(gotPath, "/nexus/v1/graphql"),
		"unexpected path: %s", gotPath)
	assert.Equal(t, http.MethodPost, gotMethod)
	require.Len(t, gotBody, 1)
	assert.Equal(t, "getEndpointState", gotBody[0]["operationName"])
	vars, _ := gotBody[0]["variables"].(map[string]any)
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
		map[string]*Device{}, map[string]string{}, nil, nil)
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
	require.NoError(t, c.Sensors.Update(context.Background(), devices, endpoints, nil, nil))

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

	require.NoError(t, c.Sensors.Update(context.Background(), devices, endpoints, nil, nil))

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

	require.NoError(t, c.Sensors.Update(context.Background(), devices, endpoints, nil, nil))
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

	require.NoError(t, c.Sensors.Update(context.Background(), devices, endpoints, nil, nil))
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
	notificationsBody, err := os.ReadFile("testdata/notifications/list.json")
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
	// Pin Notifications clock so the recurring-rule expansion is
	// deterministic.
	c.Notifications.now = func() time.Time {
		return time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	}

	require.NoError(t, c.Devices.Refresh(context.Background()))

	dvc, err := c.Devices.Get(context.Background(), "S-KITCHEN-001")
	require.NoError(t, err)

	require.NotNil(t, dvc.Sensors["reachability"])
	assert.Equal(t, "OK", dvc.Sensors["reachability"].Value)
	require.NotNil(t, dvc.Sensors["temperature"])
	assert.Equal(t, 21.5, dvc.Sensors["temperature"].Value)
	require.NotNil(t, dvc.Sensors["dnd"], "DnD entry should be plumbed in by sensorsHook")
	assert.Equal(t, true, dvc.Sensors["dnd"].Value)
	require.NotNil(t, dvc.Notifications["Alarm"],
		"Alarm should be plumbed in by sensorsHook (TIMERS_AND_ALARMS cap)")
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

func TestSensorsService_Update_NotificationsCapabilityFilter(t *testing.T) {
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
		Capabilities: []string{"REMINDERS", "TIMERS_AND_ALARMS"},
		Sensors:      map[string]*DeviceSensor{},
	}
	noCaps := &Device{
		SerialNumber: "S-NOCAPS-001",
		DeviceType:   "A18O6U1U9KIYDM",
		Capabilities: nil,
		Sensors:      map[string]*DeviceSensor{},
	}
	devices := map[string]*Device{
		kitchen.SerialNumber: kitchen,
		noCaps.SerialNumber:  noCaps,
	}
	endpoints := map[string]string{"ep-kitchen": kitchen.SerialNumber}

	notifications := map[string]map[string]*Schedule{
		kitchen.SerialNumber: {
			"Alarm":    {Type: "Alarm", Status: "ON", Label: "wakeup"},
			"Reminder": {Type: "Reminder", Status: "ON", Label: "call mom"},
			"Timer":    {Type: "Timer", Status: "ON", Label: "tea"},
		},
		noCaps.SerialNumber: {
			"Alarm": {Type: "Alarm", Status: "ON", Label: "should not appear"},
		},
	}

	require.NoError(t, c.Sensors.Update(context.Background(), devices, endpoints, nil, notifications))

	// Kitchen has both capabilities, so all three notification types pass.
	assert.Len(t, kitchen.Notifications, 3)
	assert.Equal(t, "wakeup", kitchen.Notifications["Alarm"].Label)
	assert.Equal(t, "call mom", kitchen.Notifications["Reminder"].Label)
	assert.Equal(t, "tea", kitchen.Notifications["Timer"].Label)

	// noCaps has no capabilities, so nothing passes - even though the
	// input map had an entry for it.
	assert.Len(t, noCaps.Notifications, 0)
}
