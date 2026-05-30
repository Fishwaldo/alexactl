package alexactl

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSequenceType_IsAValid(t *testing.T) {
	assert.True(t, SeqSpeak.IsValid())
	assert.False(t, SequenceType("Not.A.Real.Sequence").IsValid())
}

func TestSequenceType_JSONRoundTrip(t *testing.T) {
	for _, st := range SequenceTypeValues() {
		raw, err := json.Marshal(st)
		require.NoError(t, err)
		var back SequenceType
		require.NoError(t, json.Unmarshal(raw, &back))
		assert.Equal(t, st, back)
	}
}

func TestMediaCommand_IsAValid(t *testing.T) {
	assert.True(t, CmdPlay.IsValid())
	assert.False(t, MediaCommand("Unknown").IsValid())
}

func TestEnums_WireValuesUnchanged(t *testing.T) {
	// Wire values must match Python; sanity-checks against silent rename.
	assert.Equal(t, "Alexa.Speak", string(SeqSpeak))
	assert.Equal(t, "AlexaAnnouncement", string(SeqAnnouncement))
	assert.Equal(t, "Alexa.DeviceControls.Volume", string(SeqVolume))
	assert.Equal(t, "PlayCommand", string(CmdPlay))
	assert.Equal(t, "PauseCommand", string(CmdPause))
}
