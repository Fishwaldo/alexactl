// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package alexactl

import (
	"context"
	"fmt"

	"github.com/Fishwaldo/alexactl/internal/amzconst"
)

// PowerControl is the typed accessor returned by Device.Power() for
// devices that expose Alexa.PowerController. Its three methods —
// On, Off, Toggle — each issue one setEndpointFeatures mutation.
// On HTTP success the corresponding Device.Sensors["powerState"]
// entry is updated optimistically; the next getEndpointState refresh
// overwrites with the canonical value.
//
// PowerControl is safe for concurrent use; it holds only pointers to
// state owned by the originating Device.
type PowerControl struct {
	device *Device
}

// Power returns a typed power-controller accessor, or nil when the
// device does not advertise Alexa.PowerController in
// AlexaInterfaces, has no EndpointID, or has no back-reference to a
// smartHomeClient (e.g. a *Device built by hand outside NewClient).
//
// Callers MUST nil-check: dvc.Power().On(ctx) panics if Power()
// returned nil. This is intentional — a silent no-op on
// non-controllable devices would mask programmer error.
func (d *Device) Power() *PowerControl {
	if d == nil || d.controller == nil || d.EndpointID == "" {
		return nil
	}
	if !d.hasInterface(amzconst.InterfacePowerController) {
		return nil
	}
	return &PowerControl{device: d}
}

// On issues a turnOn directive. On HTTP 200 with no structured error,
// Device.Sensors["powerState"].Value is set to "ON".
func (c *PowerControl) On(ctx context.Context) error {
	return c.send(ctx, "turnOn", "ON")
}

// Off issues a turnOff directive. On success, Sensors["powerState"]
// is set to "OFF".
func (c *PowerControl) Off(ctx context.Context) error {
	return c.send(ctx, "turnOff", "OFF")
}

// Toggle reads the cached Sensors["powerState"] entry and issues the
// opposite directive. If no cached state exists (or the cached value
// is neither "ON" nor "OFF"), Toggle issues turnOn — the safer default
// for a powered-off device.
func (c *PowerControl) Toggle(ctx context.Context) error {
	current := ""
	if s, ok := c.device.Sensors["powerState"]; ok && s != nil {
		current, _ = s.Value.(string)
	}
	if current == "ON" {
		return c.Off(ctx)
	}
	return c.On(ctx)
}

// send is the common path: build a single-entry FeatureControl, POST
// via smartHomeClient, translate structured errors into a wrapped
// ErrCannotControlDevice, and apply the optimistic Sensor update on
// success.
func (c *PowerControl) send(ctx context.Context, op, optimisticValue string) error {
	_, fails, err := c.device.controller.setEndpointFeatures(ctx, []FeatureControl{
		{
			EndpointID:           c.device.EndpointID,
			FeatureName:          "power",
			FeatureOperationName: op,
		},
	})
	if err != nil {
		return err // already wrapped in ErrCannotConnect by smartHomeClient
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
	// Optimistic local update — overwrite on the next refresh.
	if c.device.Sensors == nil {
		c.device.Sensors = map[string]*DeviceSensor{}
	}
	if existing, ok := c.device.Sensors["powerState"]; ok && existing != nil {
		existing.Value = optimisticValue
	} else {
		c.device.Sensors["powerState"] = &DeviceSensor{
			Name:  "powerState",
			Value: optimisticValue,
		}
	}
	return nil
}

// hasInterface reports whether the device's AlexaInterfaces slice
// includes the given interface name. Linear scan — the slice is
// small (typically 1–5 entries) so a map would cost more than it
// saves.
func (d *Device) hasInterface(name string) bool {
	for _, i := range d.AlexaInterfaces {
		if i == name {
			return true
		}
	}
	return false
}
