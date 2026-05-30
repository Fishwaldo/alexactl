// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

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

func TestDevice_Thermostat_NilWhenNoController(t *testing.T) {
	d := &Device{
		SerialNumber:    "S-1",
		EndpointID:      "ep-1",
		AlexaInterfaces: []string{"Alexa.PowerController"},
		controller:      &smartHomeClient{},
	}
	assert.Nil(t, d.Thermostat())
}

func TestThermostatControl_SetTarget_PostsTypedPayload(t *testing.T) {
	var gotPayload map[string]any
	var gotFeatureOp string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		vars, _ := body["variables"].(map[string]any)
		reqs, _ := vars["requests"].([]any)
		r0 := reqs[0].(map[string]any)
		gotFeatureOp, _ = r0["featureOperationName"].(string)
		gotPayload, _ = r0["payload"].(map[string]any)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"setEndpointFeatures":{"featureControlResponses":[{"endpointId":"ep-1","featureName":"thermostat","featureOperationName":"setTargetSetpoint","instance":null,"code":"SUCCESS"}],"errors":[]}}}`))
	}))
	t.Cleanup(srv.Close)

	wr := buildSmartHomeTestWrapper(t, srv)
	d := &Device{
		SerialNumber:    "S-1",
		EndpointID:      "ep-1",
		AlexaInterfaces: []string{amzconst.InterfaceThermostatController},
		controller:      newSmartHomeClient(wr),
		Sensors:         map[string]*DeviceSensor{},
	}

	require.NoError(t, d.Thermostat().SetTarget(context.Background(), 21.5, TemperatureScaleCelsius))
	assert.Equal(t, "setTargetSetpoint", gotFeatureOp)
	require.NotNil(t, gotPayload)
	target, ok := gotPayload["targetSetpoint"].(map[string]any)
	require.True(t, ok, "payload.targetSetpoint should be an object")
	assert.InDelta(t, 21.5, target["value"], 0.001)
	assert.Equal(t, "CELSIUS", target["scale"])

	// Optimistic update.
	require.NotNil(t, d.Sensors["targetSetpoint"])
	assert.InDelta(t, 21.5, d.Sensors["targetSetpoint"].Value, 0.001)
	assert.Equal(t, "CELSIUS", d.Sensors["targetSetpoint"].Scale)
}

func TestThermostatControl_AdjustTarget_PostsDelta(t *testing.T) {
	var gotPayload map[string]any
	var gotFeatureOp string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		vars, _ := body["variables"].(map[string]any)
		reqs, _ := vars["requests"].([]any)
		r0 := reqs[0].(map[string]any)
		gotFeatureOp, _ = r0["featureOperationName"].(string)
		gotPayload, _ = r0["payload"].(map[string]any)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"setEndpointFeatures":{"featureControlResponses":[{"endpointId":"ep-1","featureName":"thermostat","featureOperationName":"adjustTargetSetpoint","instance":null,"code":"SUCCESS"}],"errors":[]}}}`))
	}))
	t.Cleanup(srv.Close)

	wr := buildSmartHomeTestWrapper(t, srv)
	d := &Device{
		SerialNumber:    "S-1",
		EndpointID:      "ep-1",
		AlexaInterfaces: []string{amzconst.InterfaceThermostatController},
		controller:      newSmartHomeClient(wr),
		Sensors:         map[string]*DeviceSensor{},
	}

	require.NoError(t, d.Thermostat().AdjustTarget(context.Background(), -1.5, TemperatureScaleCelsius))
	assert.Equal(t, "adjustTargetSetpoint", gotFeatureOp)
	require.NotNil(t, gotPayload)
	delta, ok := gotPayload["targetSetpointDelta"].(map[string]any)
	require.True(t, ok, "payload.targetSetpointDelta should be an object")
	assert.InDelta(t, -1.5, delta["value"], 0.001)
	assert.Equal(t, "CELSIUS", delta["scale"])
}

func TestThermostatControl_SetMode_PostsTypedPayload(t *testing.T) {
	var gotPayload map[string]any
	var gotFeatureOp string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		vars, _ := body["variables"].(map[string]any)
		reqs, _ := vars["requests"].([]any)
		r0 := reqs[0].(map[string]any)
		gotFeatureOp, _ = r0["featureOperationName"].(string)
		gotPayload, _ = r0["payload"].(map[string]any)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"setEndpointFeatures":{"featureControlResponses":[{"endpointId":"ep-1","featureName":"thermostat","featureOperationName":"setThermostatMode","instance":null,"code":"SUCCESS"}],"errors":[]}}}`))
	}))
	t.Cleanup(srv.Close)

	wr := buildSmartHomeTestWrapper(t, srv)
	d := &Device{
		SerialNumber:    "S-1",
		EndpointID:      "ep-1",
		AlexaInterfaces: []string{amzconst.InterfaceThermostatController},
		controller:      newSmartHomeClient(wr),
		Sensors:         map[string]*DeviceSensor{},
	}

	require.NoError(t, d.Thermostat().SetMode(context.Background(), ThermostatModeCool))
	assert.Equal(t, "setThermostatMode", gotFeatureOp)
	require.NotNil(t, gotPayload)
	assert.Equal(t, "COOL", gotPayload["thermostatMode"])

	// Optimistic update.
	require.NotNil(t, d.Sensors["thermostatMode"])
	assert.Equal(t, "COOL", d.Sensors["thermostatMode"].Value)
}

func TestThermostatControl_SetMode_StructuredErrorWrapsSentinel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"setEndpointFeatures":{
		  "featureControlResponses": [],
		  "errors": [{"endpointId": "ep-1", "featureName": "thermostat", "featureOperationName": "setThermostatMode", "instance": null, "code": "MODE_NOT_SUPPORTED", "message": "device does not support COOL"}]
		}}}`))
	}))
	t.Cleanup(srv.Close)

	wr := buildSmartHomeTestWrapper(t, srv)
	d := &Device{
		SerialNumber:    "S-1",
		EndpointID:      "ep-1",
		AlexaInterfaces: []string{amzconst.InterfaceThermostatController},
		controller:      newSmartHomeClient(wr),
		Sensors:         map[string]*DeviceSensor{},
	}

	err := d.Thermostat().SetMode(context.Background(), ThermostatModeCool)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCannotControlDevice)
	assert.Contains(t, err.Error(), "MODE_NOT_SUPPORTED")
	assert.Contains(t, err.Error(), "device does not support COOL")
	assert.Nil(t, d.Sensors["thermostatMode"], "Sensors[thermostatMode] must not be written on structured error")
}

func TestThermostatControl_SetTarget_StructuredErrorWrapsSentinel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"setEndpointFeatures":{
		  "featureControlResponses": [],
		  "errors": [{"endpointId": "ep-1", "featureName": "thermostat", "featureOperationName": "setTargetSetpoint", "instance": null, "code": "INVALID_VALUE", "message": "setpoint out of range"}]
		}}}`))
	}))
	t.Cleanup(srv.Close)

	wr := buildSmartHomeTestWrapper(t, srv)
	d := &Device{
		SerialNumber:    "S-1",
		EndpointID:      "ep-1",
		AlexaInterfaces: []string{amzconst.InterfaceThermostatController},
		controller:      newSmartHomeClient(wr),
		Sensors:         map[string]*DeviceSensor{},
	}

	err := d.Thermostat().SetTarget(context.Background(), 999.0, TemperatureScaleCelsius)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCannotControlDevice)
	assert.Contains(t, err.Error(), "INVALID_VALUE")
	assert.Contains(t, err.Error(), "setpoint out of range")
	assert.Nil(t, d.Sensors["targetSetpoint"], "Sensors[targetSetpoint] must not be written on structured error")
}
