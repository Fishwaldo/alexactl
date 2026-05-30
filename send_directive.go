// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package alexactl

import (
	"context"
	"fmt"
)

// SendDirective issues an arbitrary feature-control directive via the
// same setEndpointFeatures mutation the typed accessors use. Useful
// for Alexa interfaces we haven't shipped a typed accessor for and
// for exploring unverified wire shapes (Range/Mode/Toggle
// payloads pre-typed-accessor).
//
// instance is the feature's instance identifier when the feature
// supports multiple instances per device (Mode, Range, Toggle); pass
// "" when the feature is singleton (Power, Brightness, etc.).
//
//	dvc.SendDirective(ctx, "power", "", "turnOn", nil)
//	dvc.SendDirective(ctx, "brightness", "", "setBrightness", map[string]any{"brightness": 60})
//	dvc.SendDirective(ctx, "mode", "Pure.FanSpeed", "setMode", map[string]any{"mode": "FanSpeed.High"})
//
// HISTORICAL NOTE: previously routed through the experimental
// setEndpointFeaturesV2 mutation, which Amazon gates per-customer
// ("setEndpointFeaturesV2 is not enabled for customerId=..."). The
// V1 mutation already accepts arbitrary string featureName /
// operationName values (the schema enums are validated server-side
// at the call site, not by the binding), so V1 is strictly more
// available and equivalent in capability.
//
// On structured error, returns ErrCannotControlDevice with the
// upstream code + message in the formatted error string. On transport
// failure (network, 5xx, malformed JSON), returns ErrCannotConnect.
// No optimistic local Sensors update — the directive shape isn't
// known statically.
//
// Returns ErrDeviceNotFound if the device has no EndpointID or no
// smartHomeClient back-reference (e.g. a *Device built outside the
// Client lifecycle).
func (d *Device) SendDirective(ctx context.Context, featureName, instance, operationName string, payload map[string]any) error {
	if d == nil || d.controller == nil || d.EndpointID == "" {
		return fmt.Errorf("%w: device has no EndpointID or controller", ErrDeviceNotFound)
	}

	_, fails, err := d.controller.setEndpointFeatures(ctx, []FeatureControl{{
		EndpointID:           d.EndpointID,
		FeatureName:          featureName,
		Instance:             instance,
		FeatureOperationName: operationName,
		Payload:              payload,
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
		return fmt.Errorf("%w: %s: %s/%s on %s (%s)",
			ErrCannotControlDevice, f.Code, featureName, operationName, d.SerialNumber, msg)
	}
	return nil
}
