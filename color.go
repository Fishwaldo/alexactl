package amazondevices

import (
	"context"
	"fmt"
	"math"

	"github.com/Fishwaldo/goamazondevices/internal/amzconst"
)

// Color is the HSV-encoded color value Amazon's Alexa.ColorController
// speaks natively. Hue is in degrees [0, 360); Saturation and
// Brightness are unit floats [0, 1]. Use ColorFromRGB to construct
// from an 8-bit RGB triple, ToRGB to convert back.
//
// This is the wire shape — Amazon's setColor mutation accepts
// payload {"color": {"hue": <h>, "saturation": <s>, "brightness": <b>}}
// with these exact field names and ranges.
type Color struct {
	Hue        float64
	Saturation float64
	Brightness float64
}

// ColorFromRGB converts an 8-bit-per-channel RGB triple to HSV.
// Standard HSV formula — see e.g. https://en.wikipedia.org/wiki/HSL_and_HSV.
// Pure red gives Hue=0 (not 360); pure gray gives Hue=0 and
// Saturation=0; pure black gives Brightness=0.
func ColorFromRGB(r, g, b uint8) Color {
	rf := float64(r) / 255.0
	gf := float64(g) / 255.0
	bf := float64(b) / 255.0

	maxC := math.Max(rf, math.Max(gf, bf))
	minC := math.Min(rf, math.Min(gf, bf))
	delta := maxC - minC

	var hue float64
	switch {
	case delta == 0:
		hue = 0
	case maxC == rf:
		hue = 60 * math.Mod((gf-bf)/delta, 6)
	case maxC == gf:
		hue = 60 * ((bf-rf)/delta + 2)
	default: // maxC == bf
		hue = 60 * ((rf-gf)/delta + 4)
	}
	if hue < 0 {
		hue += 360
	}

	var saturation float64
	if maxC > 0 {
		saturation = delta / maxC
	}

	return Color{Hue: hue, Saturation: saturation, Brightness: maxC}
}

// ToRGB converts HSV back to an 8-bit-per-channel RGB triple. Inverse
// of ColorFromRGB modulo ±1-per-channel rounding error (HSV→RGB is
// not exactly invertible on the 8-bit integer grid).
func (c Color) ToRGB() (uint8, uint8, uint8) {
	if c.Brightness <= 0 {
		return 0, 0, 0
	}
	h := math.Mod(c.Hue, 360)
	if h < 0 {
		h += 360
	}
	chroma := c.Brightness * c.Saturation
	hPrime := h / 60
	x := chroma * (1 - math.Abs(math.Mod(hPrime, 2)-1))
	var rf, gf, bf float64
	switch {
	case hPrime < 1:
		rf, gf, bf = chroma, x, 0
	case hPrime < 2:
		rf, gf, bf = x, chroma, 0
	case hPrime < 3:
		rf, gf, bf = 0, chroma, x
	case hPrime < 4:
		rf, gf, bf = 0, x, chroma
	case hPrime < 5:
		rf, gf, bf = x, 0, chroma
	default:
		rf, gf, bf = chroma, 0, x
	}
	m := c.Brightness - chroma
	return clampToByte(rf + m), clampToByte(gf + m), clampToByte(bf + m)
}

// clampToByte clamps a [0,1] float to [0,255] uint8 with rounding.
func clampToByte(f float64) uint8 {
	v := math.Round(f * 255)
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// ColorControl is the typed accessor returned by Device.Color() for
// devices that expose Alexa.ColorController. Currently exposes only
// Set; Adjust/Increase/Decrease are not part of Amazon's ColorController
// schema.
type ColorControl struct {
	device *Device
}

// Color returns a typed color-controller accessor, or nil when the
// device does not advertise Alexa.ColorController, has no EndpointID,
// or has no smartHomeClient back-reference. Same nil-on-no-capability
// semantics as Power() / Brightness().
func (d *Device) Color() *ColorControl {
	if d == nil || d.controller == nil || d.EndpointID == "" {
		return nil
	}
	if !d.hasInterface(amzconst.InterfaceColorController) {
		return nil
	}
	return &ColorControl{device: d}
}

// Set issues a setColor directive with the given HSV color. On HTTP
// success and no structured error, Device.Sensors["color"].Value is
// set to a map[string]any with {"hue", "saturation", "brightness"}
// matching the read-path shape from sensors.go.
func (c *ColorControl) Set(ctx context.Context, color Color) error {
	payload := map[string]any{
		"color": map[string]any{
			"hue":        color.Hue,
			"saturation": color.Saturation,
			"brightness": color.Brightness,
		},
	}
	_, fails, err := c.device.controller.setEndpointFeatures(ctx, []FeatureControl{
		{
			EndpointID:           c.device.EndpointID,
			FeatureName:          "color",
			FeatureOperationName: "setColor",
			Payload:              payload,
		},
	})
	if err != nil {
		return err
	}
	if len(fails) > 0 {
		f := fails[0]
		msg := ""
		if f.Message != nil {
			msg = *f.Message
		}
		return fmt.Errorf("%w: %s: setColor on %s (%s)",
			ErrCannotControlDevice, f.Code, c.device.SerialNumber, msg)
	}
	if c.device.Sensors == nil {
		c.device.Sensors = map[string]*DeviceSensor{}
	}
	val := map[string]any{
		"hue":        color.Hue,
		"saturation": color.Saturation,
		"brightness": color.Brightness,
	}
	if existing, ok := c.device.Sensors["color"]; ok && existing != nil {
		existing.Value = val
	} else {
		c.device.Sensors["color"] = &DeviceSensor{
			Name:  "color",
			Value: val,
		}
	}
	return nil
}
