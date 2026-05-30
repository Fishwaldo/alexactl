// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package amzconst_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Fishwaldo/alexactl/internal/amzconst"
)

func TestDisplayCategoryToKind_KnownMappings(t *testing.T) {
	cases := []struct {
		category string
		want     string // string form of Kind
	}{
		{"LIGHT", "light"},
		{"SMARTPLUG", "switch"},
		{"SWITCH", "switch"},
		{"THERMOSTAT", "thermostat"},
		{"AIR_CONDITIONER", "thermostat"},
		{"HEATER", "thermostat"},
		{"FAN", "switch"},
		{"CAMERA", "camera"},
		{"CONTACT_SENSOR", "contact_sensor"},
		{"MOTION_SENSOR", "motion_sensor"},
		{"AIR_QUALITY_MONITOR", "air_quality_monitor"},
		{"TEMPERATURE_SENSOR", "sensor"},
		{"AIR_PURIFIER", "air_purifier"},
		{"SECURITY_PANEL", "security_panel"},
		{"SCENE_TRIGGER", "scene"},
		{"OTHER", "other"},
		{"APPLICATION", "other"},
		{"COMPUTER", "other"},
		{"", "other"},
		{"SOMETHING_BRAND_NEW", "other"},
	}
	for _, c := range cases {
		t.Run(c.category, func(t *testing.T) {
			got := amzconst.DisplayCategoryToKind(c.category)
			assert.Equal(t, c.want, got, "category %q should map to %q", c.category, c.want)
		})
	}
}
