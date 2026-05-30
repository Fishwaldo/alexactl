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

func TestDevice_ColorTemperature_NilWhenNoController(t *testing.T) {
	d := &Device{
		SerialNumber:    "S-1",
		EndpointID:      "ep-1",
		AlexaInterfaces: []string{"Alexa.PowerController"},
		controller:      &smartHomeClient{},
	}
	assert.Nil(t, d.ColorTemperature())
}

func TestColorTempControl_Set_PostsTypedPayload(t *testing.T) {
	var gotPayload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		vars, _ := body["variables"].(map[string]any)
		reqs, _ := vars["requests"].([]any)
		r0 := reqs[0].(map[string]any)
		assert.Equal(t, "colorTemperature", r0["featureName"])
		assert.Equal(t, "setColorTemperature", r0["featureOperationName"])
		gotPayload, _ = r0["payload"].(map[string]any)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"setEndpointFeatures":{"featureControlResponses":[{"endpointId":"ep-1","featureName":"colorTemperature","featureOperationName":"setColorTemperature","instance":null,"code":"SUCCESS"}],"errors":[]}}}`))
	}))
	t.Cleanup(srv.Close)

	wr := buildSmartHomeTestWrapper(t, srv)
	d := &Device{
		SerialNumber:    "S-1",
		EndpointID:      "ep-1",
		AlexaInterfaces: []string{amzconst.InterfaceColorTemperatureController},
		controller:      newSmartHomeClient(wr),
		Sensors:         map[string]*DeviceSensor{},
	}

	require.NoError(t, d.ColorTemperature().Set(context.Background(), 2700))
	require.NotNil(t, gotPayload)
	assert.EqualValues(t, 2700, gotPayload["colorTemperatureInKelvin"])

	// Optimistic update: Sensors["colorTemperature"] holds the kelvin int.
	require.NotNil(t, d.Sensors["colorTemperature"])
	assert.EqualValues(t, 2700, d.Sensors["colorTemperature"].Value)
}

func TestColorTempControl_Increase_NoPayload(t *testing.T) {
	var gotFeatureOp string
	var gotPayloadField any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		vars, _ := body["variables"].(map[string]any)
		reqs, _ := vars["requests"].([]any)
		r0 := reqs[0].(map[string]any)
		gotFeatureOp, _ = r0["featureOperationName"].(string)
		gotPayloadField = r0["payload"]
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"setEndpointFeatures":{"featureControlResponses":[{"endpointId":"ep-1","featureName":"colorTemperature","featureOperationName":"increaseColorTemperature","instance":null,"code":"SUCCESS"}],"errors":[]}}}`))
	}))
	t.Cleanup(srv.Close)

	wr := buildSmartHomeTestWrapper(t, srv)
	d := &Device{
		SerialNumber:    "S-1",
		EndpointID:      "ep-1",
		AlexaInterfaces: []string{amzconst.InterfaceColorTemperatureController},
		controller:      newSmartHomeClient(wr),
		Sensors:         map[string]*DeviceSensor{},
	}

	require.NoError(t, d.ColorTemperature().Increase(context.Background()))
	assert.Equal(t, "increaseColorTemperature", gotFeatureOp)
	assert.Nil(t, gotPayloadField, "increaseColorTemperature directive must carry no payload")
}

func TestColorTempControl_Decrease_NoPayload(t *testing.T) {
	var gotFeatureOp string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		vars, _ := body["variables"].(map[string]any)
		reqs, _ := vars["requests"].([]any)
		r0 := reqs[0].(map[string]any)
		gotFeatureOp, _ = r0["featureOperationName"].(string)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"setEndpointFeatures":{"featureControlResponses":[{"endpointId":"ep-1","featureName":"colorTemperature","featureOperationName":"decreaseColorTemperature","instance":null,"code":"SUCCESS"}],"errors":[]}}}`))
	}))
	t.Cleanup(srv.Close)

	wr := buildSmartHomeTestWrapper(t, srv)
	d := &Device{
		SerialNumber:    "S-1",
		EndpointID:      "ep-1",
		AlexaInterfaces: []string{amzconst.InterfaceColorTemperatureController},
		controller:      newSmartHomeClient(wr),
		Sensors:         map[string]*DeviceSensor{},
	}

	require.NoError(t, d.ColorTemperature().Decrease(context.Background()))
	assert.Equal(t, "decreaseColorTemperature", gotFeatureOp)
}

func TestColorTempControl_Set_StructuredErrorWrapsSentinel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"setEndpointFeatures":{
		  "featureControlResponses": [],
		  "errors": [{"endpointId": "ep-1", "featureName": "colorTemperature", "featureOperationName": "setColorTemperature", "instance": null, "code": "INVALID_VALUE", "message": "kelvin out of range"}]
		}}}`))
	}))
	t.Cleanup(srv.Close)

	wr := buildSmartHomeTestWrapper(t, srv)
	d := &Device{
		SerialNumber:    "S-1",
		EndpointID:      "ep-1",
		AlexaInterfaces: []string{amzconst.InterfaceColorTemperatureController},
		controller:      newSmartHomeClient(wr),
		Sensors:         map[string]*DeviceSensor{},
	}

	err := d.ColorTemperature().Set(context.Background(), 99999)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCannotControlDevice)
	assert.Contains(t, err.Error(), "INVALID_VALUE")
	assert.Contains(t, err.Error(), "kelvin out of range")
	assert.Nil(t, d.Sensors["colorTemperature"], "Sensors[colorTemperature] must not be written on structured error")
}
