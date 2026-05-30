// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package amzconst

import (
	_ "embed"
	"encoding/json"
	"sync"
)

// DeviceCapabilitiesRegisteredKey is the legacy storage key Python
// upstream uses to mark a session as having registered capabilities.
// Mirrored here only for fidelity — Go code uses
// Session.CapabilitiesRegistered directly.
const DeviceCapabilitiesRegisteredKey = "device_capabilities_registered"

//go:embed capabilities.json
var deviceCapabilitiesRaw []byte

var deviceCapabilitiesOnce sync.Once
var deviceCapabilitiesParsed map[string]any

// DeviceCapabilities returns the static payload sent to
// PUT /v1/devices/@self/capabilities during the first push-streaming
// connect. The returned map is shared across callers and MUST NOT be
// mutated. Lazy-parsed because the JSON is ~6KB and not every
// invocation of the binary needs it.
func DeviceCapabilities() map[string]any {
	deviceCapabilitiesOnce.Do(func() {
		var parsed map[string]any
		if err := json.Unmarshal(deviceCapabilitiesRaw, &parsed); err != nil {
			// The embed is built-in at compile time and the JSON
			// is generated from a stable source — a parse failure
			// here is a build-time bug, not a runtime concern.
			panic("amzconst: capabilities.json failed to parse: " + err.Error())
		}
		deviceCapabilitiesParsed = parsed
	})
	return deviceCapabilitiesParsed
}
