package amazondevices

import (
	"context"

	"github.com/Fishwaldo/goamazondevices/internal/amzconst"
)

// RangeControl is the typed accessor returned by Device.Range(
// instance) for devices that expose Alexa.RangeController. The
// write path is INTENTIONALLY STUBBED in Phase 4e — the
// setRangeValue / adjustRangeValue wire payload shape has not been
// confirmed against real hardware, and shipping a best-guess shape
// would silently fail or behave unexpectedly on real devices.
//
// Set and Adjust both log a WARN message asking the caller to file
// a GitHub issue with a HAR capture, then return
// ErrControllerNotImplemented. No network call is attempted.
//
// The accessor (Device.Range) still gates correctly — it returns
// non-nil only for devices that advertise Alexa.RangeController via
// AlexaInterfaces (derived from sensor keys today). This lets
// callers write capability-conditional code that's already shaped
// for when the real wire path lands.
type RangeControl struct {
	device   *Device
	instance string
}

// Range returns a typed range-controller accessor, or nil when the
// device does not advertise Alexa.RangeController, has no EndpointID,
// or has no smartHomeClient back-reference.
//
// instance is the feature instance identifier (e.g. "Apparatus.
// Humidity", "Skill.FanSpeed"). Required.
//
// WRITE PATH IS STUBBED — see ErrControllerNotImplemented and the
// WARN log emitted by Set / Adjust.
func (d *Device) Range(instance string) *RangeControl {
	if d == nil || d.controller == nil || d.EndpointID == "" {
		return nil
	}
	if !d.hasInterface(amzconst.InterfaceRangeController) {
		return nil
	}
	return &RangeControl{device: d, instance: instance}
}

// Set is the stub for setRangeValue. Logs a WARN and returns
// ErrControllerNotImplemented. Does not call the network.
func (c *RangeControl) Set(ctx context.Context, value float64) error {
	c.warn(ctx, "setRangeValue")
	return ErrControllerNotImplemented
}

// Adjust is the stub for adjustRangeValue. Logs a WARN and returns
// ErrControllerNotImplemented. Does not call the network.
func (c *RangeControl) Adjust(ctx context.Context, delta float64) error {
	c.warn(ctx, "adjustRangeValue")
	return ErrControllerNotImplemented
}

// warn emits the standard "wire shape needs HAR capture" message.
// Centralised so the wording stays consistent between Set / Adjust
// (and matches ToggleControl's warning).
func (c *RangeControl) warn(ctx context.Context, op string) {
	c.device.controller.wrapper.Logger(ctx).WarnContext(ctx,
		"amazondevices: Alexa.RangeController write path is not yet wired; "+
			"the wire payload shape for setRangeValue/adjustRangeValue has "+
			"not been confirmed against real hardware. Please file an issue "+
			"at https://github.com/Fishwaldo/goamazondevices/issues with a "+
			"HAR capture of the Alexa web UI sending the directive against "+
			"your device — see issue template 'Phase 4e wire capture'.",
		"interface", amzconst.InterfaceRangeController,
		"operation", op,
		"device", c.device.SerialNumber,
		"instance", c.instance)
}
