// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package alexactl

import (
	"context"

	"github.com/Fishwaldo/alexactl/internal/amzconst"
)

// ToggleControl is the typed accessor returned by Device.Toggle(
// instance) for devices that expose Alexa.ToggleController. The
// write path is INTENTIONALLY STUBBED — the toggle
// On/Off wire payload + operation-name shape has not been confirmed
// against real hardware (no devices on any test account expose a
// callable ToggleController). Rather than ship a best-guess shape,
// On / Off log a WARN message asking the caller to file a GitHub
// issue with a HAR capture, then return ErrControllerNotImplemented.
//
// The accessor (Device.Toggle) gates correctly — it returns non-nil
// only for devices that advertise Alexa.ToggleController via
// AlexaInterfaces. This lets callers write capability-conditional
// code that's already shape-compatible with the future real
// implementation.
type ToggleControl struct {
	device   *Device
	instance string
}

// Toggle returns a typed toggle-controller accessor, or nil when the
// device does not advertise Alexa.ToggleController, has no
// EndpointID, or has no smartHomeClient back-reference.
//
// instance is the toggle's instance identifier (e.g. "Skill.Ionizer",
// "Skill.ChildLock", "Skill.Swing"). Required.
//
// WRITE PATH IS STUBBED — see ErrControllerNotImplemented and the
// WARN log emitted by On / Off.
func (d *Device) Toggle(instance string) *ToggleControl {
	if d == nil || d.controller == nil || d.EndpointID == "" {
		return nil
	}
	if !d.hasInterface(amzconst.InterfaceToggleController) {
		return nil
	}
	return &ToggleControl{device: d, instance: instance}
}

// On is the stub for the toggle "turnOn" directive. Logs a WARN and
// returns ErrControllerNotImplemented. Does not call the network.
func (c *ToggleControl) On(ctx context.Context) error {
	c.warn(ctx, "turnOn")
	return ErrControllerNotImplemented
}

// Off is the stub for the toggle "turnOff" directive. Logs a WARN
// and returns ErrControllerNotImplemented. Does not call the network.
func (c *ToggleControl) Off(ctx context.Context) error {
	c.warn(ctx, "turnOff")
	return ErrControllerNotImplemented
}

// warn emits the standard "wire shape needs HAR capture" message.
// Centralised so wording stays consistent between On / Off (and
// matches RangeControl's warning).
func (c *ToggleControl) warn(ctx context.Context, op string) {
	c.device.controller.wrapper.Logger(ctx).WarnContext(ctx,
		"alexactl: Alexa.ToggleController write path is not yet wired; "+
			"the wire payload + operation-name shape for toggle directives has "+
			"not been confirmed against real hardware. Please file an issue at "+
			"https://github.com/Fishwaldo/alexactl/issues with a HAR "+
			"capture of the Alexa web UI sending the directive against your "+
			"device.",
		"interface", amzconst.InterfaceToggleController,
		"operation", op,
		"device", c.device.SerialNumber,
		"instance", c.instance)
}
