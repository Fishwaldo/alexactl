// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package alexactl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDevice_HasCapability(t *testing.T) {
	d := &Device{Capabilities: []string{"MICROPHONE", "AUDIO_PLAYER"}}
	assert.True(t, d.HasCapability("MICROPHONE"))
	assert.True(t, d.HasCapability("AUDIO_PLAYER"))
	assert.False(t, d.HasCapability("TIMERS_AND_ALARMS"))
}

func TestDevice_IsCluster(t *testing.T) {
	solo := &Device{ClusterMembers: map[string]string{"A": "T1"}}
	assert.False(t, solo.IsCluster())

	pair := &Device{ClusterMembers: map[string]string{"A": "T1", "B": "T2"}}
	assert.True(t, pair.IsCluster())

	empty := &Device{}
	assert.False(t, empty.IsCluster())
}

func TestDevice_NewFieldsZeroValuedByDefault(t *testing.T) {
	d := &Device{}
	assert.Empty(t, d.Kind, "Kind defaults to zero value")
	assert.Nil(t, d.DisplayCategories, "DisplayCategories nil by default")
	assert.Nil(t, d.AlexaInterfaces, "AlexaInterfaces nil by default")
	assert.Nil(t, d.Operations, "Operations nil by default")
}

func TestDeviceSensor_NewFieldsZeroValuedByDefault(t *testing.T) {
	s := &DeviceSensor{}
	assert.Nil(t, s.Configuration, "Configuration nil by default")
	assert.Nil(t, s.Operations, "Operations nil by default")
}
