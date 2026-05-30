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

// TestColor_RGBRoundTrip verifies ColorFromRGB ∘ ToRGB == identity for
// every primary, secondary, grayscale, and named-CSS-color RGB triple.
// Tolerates ±1 per channel because HSV→RGB→HSV rounds through floats.
func TestColor_RGBRoundTrip(t *testing.T) {
	cases := []struct {
		name    string
		r, g, b uint8
	}{
		{"black", 0, 0, 0},
		{"white", 255, 255, 255},
		{"red", 255, 0, 0},
		{"green", 0, 255, 0},
		{"blue", 0, 0, 255},
		{"yellow", 255, 255, 0},
		{"cyan", 0, 255, 255},
		{"magenta", 255, 0, 255},
		{"mid-gray", 128, 128, 128},
		{"orange", 255, 165, 0},
		{"purple", 128, 0, 128},
		{"teal", 0, 128, 128},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			color := ColorFromRGB(c.r, c.g, c.b)
			gotR, gotG, gotB := color.ToRGB()
			assert.InDelta(t, c.r, gotR, 1, "R round-trip")
			assert.InDelta(t, c.g, gotG, 1, "G round-trip")
			assert.InDelta(t, c.b, gotB, 1, "B round-trip")
		})
	}
}

// TestColor_ToRGB_BlackAndWhite verifies the explicit zero/full cases —
// callers rely on (0,0,0) → black and (Brightness=1,Saturation=0) → white.
func TestColor_ToRGB_BlackAndWhite(t *testing.T) {
	black := Color{Hue: 0, Saturation: 0, Brightness: 0}
	r, g, b := black.ToRGB()
	assert.Equal(t, uint8(0), r)
	assert.Equal(t, uint8(0), g)
	assert.Equal(t, uint8(0), b)

	white := Color{Hue: 0, Saturation: 0, Brightness: 1}
	r, g, b = white.ToRGB()
	assert.Equal(t, uint8(255), r)
	assert.Equal(t, uint8(255), g)
	assert.Equal(t, uint8(255), b)
}

// TestColor_ColorFromRGB_HueWraps verifies that pure red gives Hue=0,
// not Hue=360 (which is the same color but breaks downstream consumers
// that compare hue ranges).
func TestColor_ColorFromRGB_HueWraps(t *testing.T) {
	red := ColorFromRGB(255, 0, 0)
	assert.InDelta(t, 0.0, red.Hue, 0.5, "pure red Hue should be 0, not 360")
	assert.InDelta(t, 1.0, red.Saturation, 0.001)
	assert.InDelta(t, 1.0, red.Brightness, 0.001)
}

func TestDevice_Color_NilWhenNoColorController(t *testing.T) {
	d := &Device{
		SerialNumber:    "S-1",
		EndpointID:      "ep-1",
		AlexaInterfaces: []string{"Alexa.PowerController"}, // no ColorController
		controller:      &smartHomeClient{},
	}
	assert.Nil(t, d.Color())
}

func TestColorControl_Set_PostsTypedPayload(t *testing.T) {
	var gotPayload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		vars, _ := body["variables"].(map[string]any)
		reqs, _ := vars["requests"].([]any)
		r0 := reqs[0].(map[string]any)
		assert.Equal(t, "color", r0["featureName"])
		assert.Equal(t, "setColor", r0["featureOperationName"])
		gotPayload, _ = r0["payload"].(map[string]any)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"setEndpointFeatures":{"featureControlResponses":[{"endpointId":"ep-1","featureName":"color","featureOperationName":"setColor","instance":null,"code":"SUCCESS"}],"errors":[]}}}`))
	}))
	t.Cleanup(srv.Close)

	wr := buildSmartHomeTestWrapper(t, srv)
	d := &Device{
		SerialNumber:    "S-1",
		EndpointID:      "ep-1",
		AlexaInterfaces: []string{amzconst.InterfaceColorController},
		controller:      newSmartHomeClient(wr),
		Sensors:         map[string]*DeviceSensor{},
	}

	in := Color{Hue: 348.0, Saturation: 0.91, Brightness: 1.0}
	require.NoError(t, d.Color().Set(context.Background(), in))

	require.NotNil(t, gotPayload)
	colorPayload, ok := gotPayload["color"].(map[string]any)
	require.True(t, ok, "payload.color should be an object")
	assert.InDelta(t, 348.0, colorPayload["hue"], 0.001)
	assert.InDelta(t, 0.91, colorPayload["saturation"], 0.001)
	assert.InDelta(t, 1.0, colorPayload["brightness"], 0.001)

	// Optimistic local update: Sensors["color"] now holds the
	// same Color struct (as map[string]any per the read path).
	require.NotNil(t, d.Sensors["color"])
	got, ok := d.Sensors["color"].Value.(map[string]any)
	require.True(t, ok, "Sensors[color].Value should be a map")
	assert.InDelta(t, 348.0, got["hue"], 0.001)
	assert.InDelta(t, 0.91, got["saturation"], 0.001)
	assert.InDelta(t, 1.0, got["brightness"], 0.001)
}

func TestColorControl_Set_StructuredErrorWrapsSentinel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"setEndpointFeatures":{
		  "featureControlResponses": [],
		  "errors": [{"endpointId": "ep-1", "featureName": "color", "featureOperationName": "setColor", "instance": null, "code": "INVALID_VALUE", "message": "saturation out of range"}]
		}}}`))
	}))
	t.Cleanup(srv.Close)

	wr := buildSmartHomeTestWrapper(t, srv)
	d := &Device{
		SerialNumber:    "S-1",
		EndpointID:      "ep-1",
		AlexaInterfaces: []string{amzconst.InterfaceColorController},
		controller:      newSmartHomeClient(wr),
		Sensors:         map[string]*DeviceSensor{},
	}

	err := d.Color().Set(context.Background(), Color{Hue: 100, Saturation: 1.5, Brightness: 1})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCannotControlDevice)
	assert.Contains(t, err.Error(), "INVALID_VALUE")
	assert.Contains(t, err.Error(), "saturation out of range")
	// No optimistic update on error.
	assert.Nil(t, d.Sensors["color"], "Sensors[color] must not be written on structured error")
}
