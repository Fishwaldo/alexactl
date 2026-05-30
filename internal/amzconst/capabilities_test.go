// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package amzconst

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeviceCapabilities_HasExpectedShape(t *testing.T) {
	caps := DeviceCapabilities()
	require.NotNil(t, caps)
	assert.Contains(t, caps, "legacyFlags")
	assert.Contains(t, caps, "envelopeVersion")
	assert.Contains(t, caps, "capabilities")

	flags, ok := caps["legacyFlags"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, flags["SUPPORTS_COMMS"])

	capList, ok := caps["capabilities"].([]any)
	require.True(t, ok)
	assert.NotEmpty(t, capList,
		"capabilities array must have at least one AlexaInterface")
}

func TestDeviceCapabilities_IsCached(t *testing.T) {
	a := DeviceCapabilities()
	// Mutating the returned map must be visible on the next call —
	// proves sync.Once caches the parse and every caller shares
	// the same backing storage.
	a["mutated"] = true
	// Cleanup runs even if the assertion below panics, so the
	// shared singleton doesn't leak the mutation to other tests.
	t.Cleanup(func() { delete(a, "mutated") })
	b := DeviceCapabilities()
	assert.Equal(t, true, b["mutated"],
		"DeviceCapabilities should return the cached map; mutation on one return is visible on the next")
}
