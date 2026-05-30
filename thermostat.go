// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package alexactl

import (
	"context"
	"fmt"

	"github.com/Fishwaldo/alexactl/internal/amzconst"
)

// ThermostatControl is the typed accessor returned by Device.Thermostat()
// for devices that expose Alexa.ThermostatController. Exposes three
// methods: SetTarget for absolute setpoint, AdjustTarget for signed
// delta, SetMode for thermostat mode (Heat/Cool/Auto/Off/Eco/EmHeat).
//
// Per-device support varies — not every thermostat supports every
// mode. Querying the configuration first (Sensors["thermostatMode"].
// Configuration["supportedModes"], populated by the deep inspect query)
// is the recommended way to discover supported modes before calling
// SetMode.
type ThermostatControl struct {
	device *Device
}

// Thermostat returns a typed thermostat-controller accessor, or nil
// when the device does not advertise Alexa.ThermostatController, has
// no EndpointID, or has no smartHomeClient back-reference.
func (d *Device) Thermostat() *ThermostatControl {
	if d == nil || d.controller == nil || d.EndpointID == "" {
		return nil
	}
	if !d.hasInterface(amzconst.InterfaceThermostatController) {
		return nil
	}
	return &ThermostatControl{device: d}
}

// SetTarget issues a setTargetSetpoint directive at the given
// absolute temperature in the given scale. On success,
// Sensors["targetSetpoint"].Value is set to value and .Scale to the
// scale's wire string ("CELSIUS" / "FAHRENHEIT" / "KELVIN").
func (c *ThermostatControl) SetTarget(ctx context.Context, value float64, scale TemperatureScale) error {
	payload := map[string]any{
		"targetSetpoint": map[string]any{
			"value": value,
			"scale": string(scale),
		},
	}
	_, fails, err := c.device.controller.setEndpointFeatures(ctx, []FeatureControl{
		{
			EndpointID:           c.device.EndpointID,
			FeatureName:          "thermostat",
			FeatureOperationName: "setTargetSetpoint",
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
		return fmt.Errorf("%w: %s: setTargetSetpoint=%.1f%s on %s (%s)",
			ErrCannotControlDevice, f.Code, value, scale, c.device.SerialNumber, msg)
	}
	if c.device.Sensors == nil {
		c.device.Sensors = map[string]*DeviceSensor{}
	}
	if existing, ok := c.device.Sensors["targetSetpoint"]; ok && existing != nil {
		existing.Value = value
		existing.Scale = string(scale)
	} else {
		c.device.Sensors["targetSetpoint"] = &DeviceSensor{
			Name:  "targetSetpoint",
			Value: value,
			Scale: string(scale),
		}
	}
	return nil
}

// AdjustTarget issues an adjustTargetSetpoint directive with a
// signed delta in the given scale. No optimistic update — the
// resulting absolute value depends on the device's actual current
// setpoint, which we don't read synchronously. The next sensor
// refresh reflects the new absolute value.
func (c *ThermostatControl) AdjustTarget(ctx context.Context, delta float64, scale TemperatureScale) error {
	payload := map[string]any{
		"targetSetpointDelta": map[string]any{
			"value": delta,
			"scale": string(scale),
		},
	}
	_, fails, err := c.device.controller.setEndpointFeatures(ctx, []FeatureControl{
		{
			EndpointID:           c.device.EndpointID,
			FeatureName:          "thermostat",
			FeatureOperationName: "adjustTargetSetpoint",
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
		return fmt.Errorf("%w: %s: adjustTargetSetpoint=%+.1f%s on %s (%s)",
			ErrCannotControlDevice, f.Code, delta, scale, c.device.SerialNumber, msg)
	}
	return nil
}

// SetMode issues a setThermostatMode directive. The payload's
// thermostatMode field carries the wire string ("HEAT" / "COOL" /
// "AUTO" / "OFF" / "ECO" / "EM_HEAT"). On success,
// Sensors["thermostatMode"].Value is set to the wire string.
func (c *ThermostatControl) SetMode(ctx context.Context, mode ThermostatMode) error {
	_, fails, err := c.device.controller.setEndpointFeatures(ctx, []FeatureControl{
		{
			EndpointID:           c.device.EndpointID,
			FeatureName:          "thermostat",
			FeatureOperationName: "setThermostatMode",
			Payload:              map[string]any{"thermostatMode": string(mode)},
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
		return fmt.Errorf("%w: %s: setThermostatMode=%s on %s (%s)",
			ErrCannotControlDevice, f.Code, mode, c.device.SerialNumber, msg)
	}
	if c.device.Sensors == nil {
		c.device.Sensors = map[string]*DeviceSensor{}
	}
	if existing, ok := c.device.Sensors["thermostatMode"]; ok && existing != nil {
		existing.Value = string(mode)
	} else {
		c.device.Sensors["thermostatMode"] = &DeviceSensor{
			Name:  "thermostatMode",
			Value: string(mode),
		}
	}
	return nil
}
