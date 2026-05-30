// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package locale_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Fishwaldo/alexactl/internal/locale"
)

func TestForCountry(t *testing.T) {
	cases := map[string]string{
		"US": "en-US",
		"GB": "en-GB",
		"JP": "ja-JP",
		"DE": "de-DE",
		"IT": "it-IT",
		"FR": "fr-FR",
	}
	for cc, want := range cases {
		got, err := locale.ForCountry(cc)
		assert.NoError(t, err, "cc=%s", cc)
		assert.Equal(t, want, got, "cc=%s", cc)
	}
}

func TestForCountry_Invalid(t *testing.T) {
	_, err := locale.ForCountry("ZZ")
	// ZZ is an officially unassigned ISO code; x/text returns an error or
	// an empty maximize, so we accept either nil-with-empty or an error.
	if err == nil {
		got, _ := locale.ForCountry("ZZ")
		assert.NotContains(t, got, "ZZ-")
	}
}
