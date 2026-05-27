package amazondevices

import (
	"context"
	"fmt"

	"github.com/Fishwaldo/goamazondevices/internal/amzconst"
)

// ColorTempControl is the typed accessor returned by
// Device.ColorTemperature() for devices that expose
// Alexa.ColorTemperatureController. Set takes an absolute kelvin
// value; Increase/Decrease step in device-defined increments (no
// payload — the device decides the step size).
type ColorTempControl struct {
	device *Device
}

// ColorTemperature returns a typed color-temperature accessor, or nil
// when the device does not advertise Alexa.ColorTemperatureController,
// has no EndpointID, or has no smartHomeClient back-reference.
func (d *Device) ColorTemperature() *ColorTempControl {
	if d == nil || d.controller == nil || d.EndpointID == "" {
		return nil
	}
	if !d.hasInterface(amzconst.InterfaceColorTemperatureController) {
		return nil
	}
	return &ColorTempControl{device: d}
}

// Set issues a setColorTemperature directive at the given absolute
// kelvin value. On success, Sensors["colorTemperature"].Value is set
// to kelvin.
//
// Inputs are NOT clamped — Amazon's per-device supported range varies
// (typical incandescent: ~2700K; daylight: ~6500K) and the device
// will respond with INVALID_VALUE if asked for an unreachable
// temperature, which surfaces as ErrCannotControlDevice.
func (c *ColorTempControl) Set(ctx context.Context, kelvin int) error {
	_, fails, err := c.device.controller.setEndpointFeatures(ctx, []FeatureControl{
		{
			EndpointID:           c.device.EndpointID,
			FeatureName:          "colorTemperature",
			FeatureOperationName: "setColorTemperature",
			Payload:              map[string]any{"colorTemperatureInKelvin": kelvin},
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
		return fmt.Errorf("%w: %s: setColorTemperature=%dK on %s (%s)",
			ErrCannotControlDevice, f.Code, kelvin, c.device.SerialNumber, msg)
	}
	if c.device.Sensors == nil {
		c.device.Sensors = map[string]*DeviceSensor{}
	}
	if existing, ok := c.device.Sensors["colorTemperature"]; ok && existing != nil {
		existing.Value = kelvin
	} else {
		c.device.Sensors["colorTemperature"] = &DeviceSensor{
			Name:  "colorTemperature",
			Value: kelvin,
		}
	}
	return nil
}

// Increase issues an increaseColorTemperature directive. The device
// chooses the step size; we don't get the new value back synchronously,
// so no optimistic update is applied. The next sensor refresh
// reflects the new value.
func (c *ColorTempControl) Increase(ctx context.Context) error {
	return c.step(ctx, "increaseColorTemperature")
}

// Decrease issues a decreaseColorTemperature directive. Same
// caveats as Increase — no optimistic update.
func (c *ColorTempControl) Decrease(ctx context.Context) error {
	return c.step(ctx, "decreaseColorTemperature")
}

// step issues a no-payload directive. Shared by Increase and Decrease
// because the only difference is the operation name.
func (c *ColorTempControl) step(ctx context.Context, op string) error {
	_, fails, err := c.device.controller.setEndpointFeatures(ctx, []FeatureControl{
		{
			EndpointID:           c.device.EndpointID,
			FeatureName:          "colorTemperature",
			FeatureOperationName: op,
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
		return fmt.Errorf("%w: %s: %s on %s (%s)",
			ErrCannotControlDevice, f.Code, op, c.device.SerialNumber, msg)
	}
	return nil
}
