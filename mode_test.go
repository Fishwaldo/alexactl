package alexactl

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Fishwaldo/alexactl/internal/amzconst"
)

func TestDevice_Mode_NilWhenNoModeController(t *testing.T) {
	d := &Device{
		SerialNumber:    "S-1",
		EndpointID:      "ep-1",
		AlexaInterfaces: []string{"Alexa.PowerController"}, // no ModeController
		controller:      &smartHomeClient{},
	}
	assert.Nil(t, d.Mode("Pure.FanSpeed"))
}

func TestDevice_Mode_NilWhenNoController(t *testing.T) {
	d := &Device{
		SerialNumber:    "S-1",
		EndpointID:      "ep-1",
		AlexaInterfaces: []string{amzconst.InterfaceModeController},
		// controller intentionally nil
	}
	assert.Nil(t, d.Mode("Pure.FanSpeed"))
}

func TestDevice_Mode_NilWhenNoEndpointID(t *testing.T) {
	d := &Device{
		SerialNumber:    "S-1",
		AlexaInterfaces: []string{amzconst.InterfaceModeController},
		controller:      &smartHomeClient{},
	}
	assert.Nil(t, d.Mode("Pure.FanSpeed"))
}

func TestModeControl_Set_PostsTypedPayloadAndUpdatesCanonicalSensor(t *testing.T) {
	var gotPayload map[string]any
	var gotInstance string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		vars, _ := body["variables"].(map[string]any)
		reqs, _ := vars["requests"].([]any)
		r0 := reqs[0].(map[string]any)
		assert.Equal(t, "mode", r0["featureName"])
		assert.Equal(t, "setMode", r0["featureOperationName"])
		gotInstance, _ = r0["instance"].(string)
		gotPayload, _ = r0["payload"].(map[string]any)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"setEndpointFeatures":{"featureControlResponses":[{"endpointId":"ep-1","featureName":"mode","featureOperationName":"setMode","instance":"Apparatus.FanSpeed","code":"SUCCESS"}],"errors":[]}}}`))
	}))
	t.Cleanup(srv.Close)

	wr := buildSmartHomeTestWrapper(t, srv)
	d := &Device{
		SerialNumber:    "S-1",
		EndpointID:      "ep-1",
		AlexaInterfaces: []string{amzconst.InterfaceModeController},
		controller:      newSmartHomeClient(wr),
		Sensors: map[string]*DeviceSensor{
			"fanSpeed": {Name: "fanSpeed", Value: "Auto"},
		},
	}

	require.NoError(t, d.Mode("Apparatus.FanSpeed").Set(context.Background(), "L3"))
	assert.Equal(t, "Apparatus.FanSpeed", gotInstance,
		"instance must be passed via request.instance, NOT inside the payload")
	require.NotNil(t, gotPayload)
	assert.Equal(t, "L3", gotPayload["mode"], "setMode payload is {\"mode\": <value>} flat (confirmed against live device)")

	// Apparatus.FanSpeed maps to canonical sensor key "fanSpeed" via
	// amzconst.ModeInstances — optimistic update lands there.
	require.NotNil(t, d.Sensors["fanSpeed"])
	assert.Equal(t, "L3", d.Sensors["fanSpeed"].Value)
}

func TestModeControl_Set_UnknownInstanceUsesPrefixedSensorKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"setEndpointFeatures":{"featureControlResponses":[{"endpointId":"ep-1","featureName":"mode","featureOperationName":"setMode","instance":"Genesis.ColorPalette","code":"SUCCESS"}],"errors":[]}}}`))
	}))
	t.Cleanup(srv.Close)

	wr := buildSmartHomeTestWrapper(t, srv)
	d := &Device{
		SerialNumber:    "S-1",
		EndpointID:      "ep-1",
		AlexaInterfaces: []string{amzconst.InterfaceModeController},
		controller:      newSmartHomeClient(wr),
		Sensors:         map[string]*DeviceSensor{},
	}

	require.NoError(t, d.Mode("Genesis.ColorPalette").Set(context.Background(), "Spectrum"))
	// Unknown instance — sensor key is "mode:<instance>".
	require.NotNil(t, d.Sensors["mode:Genesis.ColorPalette"])
	assert.Equal(t, "Spectrum", d.Sensors["mode:Genesis.ColorPalette"].Value)
}

func TestModeControl_Adjust_PostsModeDeltaPayload(t *testing.T) {
	var gotPayload map[string]any
	var gotOp string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		vars, _ := body["variables"].(map[string]any)
		reqs, _ := vars["requests"].([]any)
		r0 := reqs[0].(map[string]any)
		gotOp, _ = r0["featureOperationName"].(string)
		gotPayload, _ = r0["payload"].(map[string]any)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"setEndpointFeatures":{"featureControlResponses":[{"endpointId":"ep-1","featureName":"mode","featureOperationName":"adjustMode","instance":"Apparatus.FanSpeed","code":"SUCCESS"}],"errors":[]}}}`))
	}))
	t.Cleanup(srv.Close)

	wr := buildSmartHomeTestWrapper(t, srv)
	d := &Device{
		SerialNumber:    "S-1",
		EndpointID:      "ep-1",
		AlexaInterfaces: []string{amzconst.InterfaceModeController},
		controller:      newSmartHomeClient(wr),
		Sensors: map[string]*DeviceSensor{
			"fanSpeed": {Name: "fanSpeed", Value: "Auto"},
		},
	}

	require.NoError(t, d.Mode("Apparatus.FanSpeed").Adjust(context.Background(), 1))
	assert.Equal(t, "adjustMode", gotOp)
	require.NotNil(t, gotPayload)
	assert.EqualValues(t, 1, gotPayload["modeDelta"], "adjustMode payload is {\"modeDelta\": <int>}")

	// Adjust must NOT optimistic-update — the resulting absolute mode
	// value depends on the device's current value, which we don't read
	// synchronously.
	assert.Equal(t, "Auto", d.Sensors["fanSpeed"].Value,
		"Adjust should not optimistically modify Sensors[<key>].Value")
}

func TestModeControl_Set_StructuredErrorWrapsSentinel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"setEndpointFeatures":{
		  "featureControlResponses": [],
		  "errors": [{"endpointId":"ep-1","featureName":"mode","featureOperationName":"setMode","instance":"Apparatus.FanSpeed","code":"INVALID_VALUE","message":"unsupported mode value"}]
		}}}`))
	}))
	t.Cleanup(srv.Close)

	wr := buildSmartHomeTestWrapper(t, srv)
	d := &Device{
		SerialNumber:    "S-1",
		EndpointID:      "ep-1",
		AlexaInterfaces: []string{amzconst.InterfaceModeController},
		controller:      newSmartHomeClient(wr),
		Sensors:         map[string]*DeviceSensor{},
	}

	err := d.Mode("Apparatus.FanSpeed").Set(context.Background(), "Nonsense.Mode")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCannotControlDevice)
	assert.Contains(t, err.Error(), "INVALID_VALUE")
	assert.Contains(t, err.Error(), "unsupported mode value")
	// No optimistic update on error.
	assert.Nil(t, d.Sensors["fanSpeed"])
}
