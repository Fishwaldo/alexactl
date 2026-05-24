package amazondevices

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
