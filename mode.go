package alexactl

import (
	"context"
	"fmt"

	"github.com/Fishwaldo/alexactl/internal/amzconst"
)

// ModeControl is the typed accessor returned by Device.Mode(instance)
// for devices that expose Alexa.ModeController. A single device can
// expose multiple Mode instances (a Sensibo air-conditioner has
// Apparatus.FanSpeed AND Apparatus.FanDirection, both addressed
// through ModeControl with different instance arguments).
//
// Set posts {"mode": <wire-string>} via the V1 setEndpointFeatures
// mutation. The valid wire-string values per instance are advertised
// by the device in Sensors[<key>].Configuration["modeOptions"] when
// the deep query has populated it (see ModeConfiguration parsing).
//
// Adjust posts {"modeDelta": <int>} for instances that support
// stepping. No optimistic update on Adjust because the resulting
// absolute mode depends on the device's current value.
type ModeControl struct {
	device   *Device
	instance string
}

// Mode returns a typed mode-controller accessor, or nil when the
// device does not advertise Alexa.ModeController, has no EndpointID,
// or has no smartHomeClient back-reference. Callers MUST nil-check.
//
// instance is the feature instance identifier (e.g. "Pure.FanSpeed",
// "Apparatus.FanDirection", "Genesis.ColorPalette"). It's required —
// ModeController is always instance-scoped on the wire.
func (d *Device) Mode(instance string) *ModeControl {
	if d == nil || d.controller == nil || d.EndpointID == "" {
		return nil
	}
	if !d.hasInterface(amzconst.InterfaceModeController) {
		return nil
	}
	return &ModeControl{device: d, instance: instance}
}

// Set issues a setMode directive with the given mode value (e.g.
// "FanSpeed.High", "Spectrum"). On success, the matching sensor in
// Device.Sensors is updated optimistically — the sensor key is the
// canonical name from amzconst.ModeInstances if the instance is
// known, else "mode:<instance>" matching buildSensor's fallback.
//
// Validates nothing client-side: the device's structured-error
// response surfaces invalid values via ErrCannotControlDevice with
// the upstream INVALID_VALUE code in the message.
func (c *ModeControl) Set(ctx context.Context, value string) error {
	_, fails, err := c.device.controller.setEndpointFeatures(ctx, []FeatureControl{{
		EndpointID:           c.device.EndpointID,
		FeatureName:          "mode",
		FeatureOperationName: "setMode",
		Instance:             c.instance,
		Payload:              map[string]any{"mode": value},
	}})
	if err != nil {
		return err
	}
	if len(fails) > 0 {
		f := fails[0]
		msg := ""
		if f.Message != nil {
			msg = *f.Message
		}
		return fmt.Errorf("%w: %s: setMode(%s=%q) on %s (%s)",
			ErrCannotControlDevice, f.Code, c.instance, value, c.device.SerialNumber, msg)
	}
	c.optimisticUpdate(value)
	return nil
}

// Adjust issues an adjustMode directive with the given signed delta.
// No optimistic update — the resulting absolute mode depends on the
// device's current value, which we don't read synchronously.
func (c *ModeControl) Adjust(ctx context.Context, delta int) error {
	_, fails, err := c.device.controller.setEndpointFeatures(ctx, []FeatureControl{{
		EndpointID:           c.device.EndpointID,
		FeatureName:          "mode",
		FeatureOperationName: "adjustMode",
		Instance:             c.instance,
		Payload:              map[string]any{"modeDelta": delta},
	}})
	if err != nil {
		return err
	}
	if len(fails) > 0 {
		f := fails[0]
		msg := ""
		if f.Message != nil {
			msg = *f.Message
		}
		return fmt.Errorf("%w: %s: adjustMode(%s, delta=%+d) on %s (%s)",
			ErrCannotControlDevice, f.Code, c.instance, delta, c.device.SerialNumber, msg)
	}
	return nil
}

// optimisticUpdate writes the given mode value into the corresponding
// DeviceSensor, creating it if missing. Sensor-key resolution mirrors
// buildSensor's instance-rewrite logic: canonical entry from
// amzconst.ModeInstances wins; otherwise "mode:<instance>" is used.
func (c *ModeControl) optimisticUpdate(value string) {
	key := "mode:" + c.instance
	if canonical, ok := amzconst.ModeInstances[c.instance]; ok {
		key = canonical.Name
	}
	if c.device.Sensors == nil {
		c.device.Sensors = map[string]*DeviceSensor{}
	}
	if existing, ok := c.device.Sensors[key]; ok && existing != nil {
		existing.Value = value
	} else {
		c.device.Sensors[key] = &DeviceSensor{
			Name:  key,
			Value: value,
		}
	}
}
