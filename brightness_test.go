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

func TestDevice_Brightness_NilWhenNoBrightnessController(t *testing.T) {
	d := &Device{
		SerialNumber:    "S-1",
		EndpointID:      "ep-1",
		AlexaInterfaces: []string{"Alexa.PowerController"}, // power only, no brightness
		controller:      &smartHomeClient{},
	}
	assert.Nil(t, d.Brightness())
}

func TestBrightnessControl_Set_PostsTypedPayload(t *testing.T) {
	var gotPayload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		vars, _ := body["variables"].(map[string]any)
		reqs, _ := vars["requests"].([]any)
		r0 := reqs[0].(map[string]any)
		assert.Equal(t, "brightness", r0["featureName"])
		assert.Equal(t, "setBrightness", r0["featureOperationName"])
		gotPayload, _ = r0["payload"].(map[string]any)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"setEndpointFeatures":{"featureControlResponses":[{"endpointId":"ep-1","featureName":"brightness","featureOperationName":"setBrightness","instance":null,"code":"SUCCESS"}],"errors":[]}}}`))
	}))
	t.Cleanup(srv.Close)

	wr := buildSmartHomeTestWrapper(t, srv)
	d := &Device{
		SerialNumber:    "S-1",
		EndpointID:      "ep-1",
		AlexaInterfaces: []string{amzconst.InterfaceBrightnessController},
		controller:      newSmartHomeClient(wr),
		Sensors:         map[string]*DeviceSensor{},
	}

	require.NoError(t, d.Brightness().Set(context.Background(), 75))
	require.NotNil(t, gotPayload)
	assert.EqualValues(t, 75, gotPayload["brightness"])

	// Optimistic update: Sensors["brightness"] should now read 75.
	require.NotNil(t, d.Sensors["brightness"])
	assert.EqualValues(t, 75, d.Sensors["brightness"].Value)
}

func TestBrightnessControl_Set_ClampsInputRange(t *testing.T) {
	cases := []struct {
		in, want int
	}{
		{-10, 0},
		{0, 0},
		{50, 50},
		{100, 100},
		{150, 100},
	}
	for _, c := range cases {
		t.Run("", func(t *testing.T) {
			var gotPayload map[string]any
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				vars, _ := body["variables"].(map[string]any)
				reqs, _ := vars["requests"].([]any)
				r0 := reqs[0].(map[string]any)
				gotPayload, _ = r0["payload"].(map[string]any)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"data":{"setEndpointFeatures":{"featureControlResponses":[{"endpointId":"ep-1","featureName":"brightness","featureOperationName":"setBrightness","instance":null,"code":"SUCCESS"}],"errors":[]}}}`))
			}))
			t.Cleanup(srv.Close)

			wr := buildSmartHomeTestWrapper(t, srv)
			d := &Device{
				SerialNumber:    "S-1",
				EndpointID:      "ep-1",
				AlexaInterfaces: []string{amzconst.InterfaceBrightnessController},
				controller:      newSmartHomeClient(wr),
				Sensors:         map[string]*DeviceSensor{},
			}

			require.NoError(t, d.Brightness().Set(context.Background(), c.in))
			require.NotNil(t, gotPayload, "in=%d", c.in)
			assert.EqualValues(t, c.want, gotPayload["brightness"], "in=%d", c.in)
		})
	}
}

func TestBrightnessControl_Adjust_PostsTypedPayload(t *testing.T) {
	var gotFeatureOp string
	var gotPayload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		vars, _ := body["variables"].(map[string]any)
		reqs, _ := vars["requests"].([]any)
		r0 := reqs[0].(map[string]any)
		gotFeatureOp, _ = r0["featureOperationName"].(string)
		gotPayload, _ = r0["payload"].(map[string]any)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"setEndpointFeatures":{"featureControlResponses":[{"endpointId":"ep-1","featureName":"brightness","featureOperationName":"adjustBrightness","instance":null,"code":"SUCCESS"}],"errors":[]}}}`))
	}))
	t.Cleanup(srv.Close)

	wr := buildSmartHomeTestWrapper(t, srv)
	d := &Device{
		SerialNumber:    "S-1",
		EndpointID:      "ep-1",
		AlexaInterfaces: []string{amzconst.InterfaceBrightnessController},
		controller:      newSmartHomeClient(wr),
		Sensors: map[string]*DeviceSensor{
			"brightness": {Name: "brightness", Value: 40},
		},
	}

	require.NoError(t, d.Brightness().Adjust(context.Background(), -10))
	assert.Equal(t, "adjustBrightness", gotFeatureOp)
	require.NotNil(t, gotPayload)
	assert.EqualValues(t, -10, gotPayload["brightnessDelta"])

	// Optimistic update: 40 + (-10) = 30, clamped to [0,100].
	assert.EqualValues(t, 30, d.Sensors["brightness"].Value)
}
