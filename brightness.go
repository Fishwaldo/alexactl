package amazondevices

import (
	"context"
	"fmt"

	"github.com/Fishwaldo/goamazondevices/internal/amzconst"
)

// BrightnessControl is the typed accessor returned by Device.Brightness()
// for devices that expose Alexa.BrightnessController. Set takes an
// absolute percentage [0..100]; Adjust takes a signed delta in the
// same units. Both clamp the local optimistic update to the valid
// range; out-of-range inputs to Set are clamped before sending,
// matching upstream's tolerance for out-of-range inputs.
type BrightnessControl struct {
	device *Device
}

// Brightness returns a typed brightness-controller accessor, or nil
// when the device does not advertise Alexa.BrightnessController, has
// no EndpointID, or has no smartHomeClient back-reference.
func (d *Device) Brightness() *BrightnessControl {
	if d == nil || d.controller == nil || d.EndpointID == "" {
		return nil
	}
	if !d.hasInterface(amzconst.InterfaceBrightnessController) {
		return nil
	}
	return &BrightnessControl{device: d}
}

// Set issues a setBrightness directive at the given absolute
// percentage. Inputs are clamped to [0,100] before sending — Amazon
// rejects out-of-range values with INVALID_VALUE, so clamping
// preserves the caller's intent without a round-trip failure.
func (c *BrightnessControl) Set(ctx context.Context, percent int) error {
	pct := clampBrightness(percent)
	_, fails, err := c.device.controller.setEndpointFeatures(ctx, []FeatureControl{
		{
			EndpointID:           c.device.EndpointID,
			FeatureName:          "brightness",
			FeatureOperationName: "setBrightness",
			Payload:              map[string]any{"brightness": pct},
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
		return fmt.Errorf("%w: %s: setBrightness=%d on %s (%s)",
			ErrCannotControlDevice, f.Code, pct, c.device.SerialNumber, msg)
	}
	if c.device.Sensors == nil {
		c.device.Sensors = map[string]*DeviceSensor{}
	}
	if existing, ok := c.device.Sensors["brightness"]; ok && existing != nil {
		existing.Value = pct
	} else {
		c.device.Sensors["brightness"] = &DeviceSensor{
			Name:  "brightness",
			Value: pct,
		}
	}
	return nil
}

// Adjust issues an adjustBrightness directive with the given signed
// delta. The local optimistic update clamps the resulting value to
// [0,100]. If the device reports no current Sensors["brightness"]
// value, Adjust still sends — Amazon resolves the absolute target
// using the device's actual current value — but skips the optimistic
// update.
func (c *BrightnessControl) Adjust(ctx context.Context, delta int) error {
	_, fails, err := c.device.controller.setEndpointFeatures(ctx, []FeatureControl{
		{
			EndpointID:           c.device.EndpointID,
			FeatureName:          "brightness",
			FeatureOperationName: "adjustBrightness",
			Payload:              map[string]any{"brightnessDelta": delta},
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
		return fmt.Errorf("%w: %s: adjustBrightness=%+d on %s (%s)",
			ErrCannotControlDevice, f.Code, delta, c.device.SerialNumber, msg)
	}
	// Optimistic update only if we have a current value to start from.
	if c.device.Sensors == nil {
		return nil
	}
	current, ok := c.device.Sensors["brightness"]
	if !ok || current == nil {
		return nil
	}
	currentInt, ok := current.Value.(int)
	if !ok {
		// Sensors round-trip the value as int; if it isn't (perhaps a test
		// fixture used float64), skip the optimistic update rather than
		// guess at the conversion.
		return nil
	}
	current.Value = clampBrightness(currentInt + delta)
	return nil
}

// clampBrightness clamps a brightness percentage to [0,100]. Inlined
// for use by Set and the Adjust optimistic update.
func clampBrightness(v int) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}
