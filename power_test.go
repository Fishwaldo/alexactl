package amazondevices

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Fishwaldo/goamazondevices/internal/amzconst"
)

func TestDevice_Power_NilWhenNoPowerController(t *testing.T) {
	d := &Device{
		SerialNumber:    "S-1",
		EndpointID:      "ep-1",
		AlexaInterfaces: []string{"Alexa.MotionSensor"}, // no PowerController
		controller:      &smartHomeClient{},
	}
	assert.Nil(t, d.Power(), "Power() must return nil when device doesn't advertise Alexa.PowerController")
}

func TestDevice_Power_NilWhenNoController(t *testing.T) {
	d := &Device{
		SerialNumber:    "S-1",
		EndpointID:      "ep-1",
		AlexaInterfaces: []string{amzconst.InterfacePowerController},
		// controller intentionally nil — e.g., a *Device built by hand in
		// a test.
	}
	assert.Nil(t, d.Power(), "Power() must return nil when device has no controller back-reference")
}

func TestDevice_Power_NilWhenNoEndpointID(t *testing.T) {
	d := &Device{
		SerialNumber:    "S-1",
		AlexaInterfaces: []string{amzconst.InterfacePowerController},
		controller:      &smartHomeClient{},
	}
	assert.Nil(t, d.Power(), "Power() must return nil when device has no EndpointID")
}

func TestPowerControl_On_Off_PostsCorrectFeatureControl(t *testing.T) {
	var lastOpName string
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		vars, _ := body["variables"].(map[string]any)
		reqs, _ := vars["requests"].([]any)
		r0 := reqs[0].(map[string]any)
		lastOpName = r0["featureOperationName"].(string)
		assert.Equal(t, "power", r0["featureName"])
		assert.Equal(t, "ep-1", r0["endpointId"])
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
		  "data": {"setEndpointFeatures": {
		    "featureControlResponses": [{"endpointId": "ep-1", "featureName": "power", "featureOperationName": "` + lastOpName + `", "instance": null, "code": "SUCCESS"}],
		    "errors": []
		  }}
		}`))
	}))
	t.Cleanup(srv.Close)

	wr := buildSmartHomeTestWrapper(t, srv)
	d := &Device{
		SerialNumber:    "S-1",
		EndpointID:      "ep-1",
		AlexaInterfaces: []string{amzconst.InterfacePowerController},
		controller:      newSmartHomeClient(wr),
		Sensors:         map[string]*DeviceSensor{},
	}

	power := d.Power()
	require.NotNil(t, power, "Power() must return non-nil for devices with Alexa.PowerController")

	require.NoError(t, power.On(context.Background()))
	assert.Equal(t, "turnOn", lastOpName)

	require.NoError(t, power.Off(context.Background()))
	assert.Equal(t, "turnOff", lastOpName)

	assert.Equal(t, int32(2), atomic.LoadInt32(&calls))
}

func TestPowerControl_StructuredErrorWrapsSentinel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"setEndpointFeatures":{
		  "featureControlResponses": [],
		  "errors": [{"endpointId": "ep-1", "featureName": "power", "featureOperationName": "turnOn", "instance": null, "code": "ENDPOINT_UNREACHABLE", "message": "device offline"}]
		}}}`))
	}))
	t.Cleanup(srv.Close)

	wr := buildSmartHomeTestWrapper(t, srv)
	d := &Device{
		SerialNumber:    "S-1",
		EndpointID:      "ep-1",
		AlexaInterfaces: []string{amzconst.InterfacePowerController},
		controller:      newSmartHomeClient(wr),
		Sensors:         map[string]*DeviceSensor{},
	}

	err := d.Power().On(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCannotControlDevice)
	assert.Contains(t, err.Error(), "ENDPOINT_UNREACHABLE")
	assert.Contains(t, err.Error(), "device offline")
	assert.True(t, strings.Contains(err.Error(), "ep-1") || strings.Contains(err.Error(), "S-1"),
		"error message should identify the failed device")
}

func TestPowerControl_Toggle_FlipsCurrentValue(t *testing.T) {
	var gotOps []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		vars, _ := body["variables"].(map[string]any)
		reqs, _ := vars["requests"].([]any)
		r0 := reqs[0].(map[string]any)
		gotOps = append(gotOps, r0["featureOperationName"].(string))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"setEndpointFeatures":{"featureControlResponses":[{"endpointId":"ep-1","featureName":"power","featureOperationName":"` + r0["featureOperationName"].(string) + `","instance":null,"code":"SUCCESS"}],"errors":[]}}}`))
	}))
	t.Cleanup(srv.Close)

	wr := buildSmartHomeTestWrapper(t, srv)
	d := &Device{
		SerialNumber:    "S-1",
		EndpointID:      "ep-1",
		AlexaInterfaces: []string{amzconst.InterfacePowerController},
		controller:      newSmartHomeClient(wr),
		Sensors: map[string]*DeviceSensor{
			"powerState": {Name: "powerState", Value: "OFF"},
		},
	}

	require.NoError(t, d.Power().Toggle(context.Background()))
	require.Len(t, gotOps, 1)
	assert.Equal(t, "turnOn", gotOps[0], "Toggle from OFF should issue turnOn")

	d.Sensors["powerState"].Value = "ON"
	require.NoError(t, d.Power().Toggle(context.Background()))
	require.Len(t, gotOps, 2)
	assert.Equal(t, "turnOff", gotOps[1], "Toggle from ON should issue turnOff")
}
