// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package alexactl

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loadFixture(t *testing.T, name string) json.RawMessage {
	t.Helper()
	raw, err := os.ReadFile("testdata/push/events/" + name)
	require.NoError(t, err)
	return raw
}

func TestDecodeRenderingUpdate_VolumeChange(t *testing.T) {
	raw := loadFixture(t, "volume_change.json")
	evts, err := DecodeRenderingUpdate(raw)
	require.NoError(t, err)
	require.Len(t, evts, 1)

	v, ok := evts[0].(*VolumeChangeEvent)
	require.True(t, ok, "expected *VolumeChangeEvent, got %T", evts[0])
	assert.Equal(t, PushVolumeChange, v.Type())
	assert.Equal(t, "G090XXXXXX1234", v.SerialNumber())
	assert.Equal(t, 42, v.Volume)
	assert.False(t, v.Muted)
	assert.NotNil(t, v.RawPayload())
	assert.Equal(t, 42.0, v.RawPayload()["volumeSetting"],
		"RawPayload should retain the JSON-decoded number as float64")
}

func TestDecodeRenderingUpdate_NotificationChange_OddSurfaces(t *testing.T) {
	raw := loadFixture(t, "notification_change.json")
	evts, err := DecodeRenderingUpdate(raw)
	require.NoError(t, err)
	require.Len(t, evts, 1)
	_, ok := evts[0].(*NotificationChangeEvent)
	assert.True(t, ok)
}

func TestDecodeRenderingUpdate_NotificationChange_EvenFiltered(t *testing.T) {
	raw := loadFixture(t, "notification_change_even.json")
	evts, err := DecodeRenderingUpdate(raw)
	require.NoError(t, err)
	assert.Empty(t, evts,
		"even notificationVersion must be dropped silently")
}

func TestDecodeRenderingUpdate_Unknown_SurfacesAsUnknownPushEvent(t *testing.T) {
	raw := loadFixture(t, "unknown.json")
	evts, err := DecodeRenderingUpdate(raw)
	require.NoError(t, err)
	require.Len(t, evts, 1)
	u, ok := evts[0].(*UnknownPushEvent)
	require.True(t, ok, "expected *UnknownPushEvent, got %T", evts[0])
	assert.Equal(t, "PUSH_DEFINITELY_NOT_REAL", u.WireType)
	assert.Equal(t, "G090XXXXXX1234", u.SerialNumber())
	assert.Equal(t, "hello-from-the-future", u.RawPayload()["diagnostic"])
}

func TestDecodeRenderingUpdate_MalformedInputs_DropSilently(t *testing.T) {
	cases := map[string]string{
		"missing renderingUpdates": `{"directive":{"payload":{}}}`,
		"empty renderingUpdates":   `{"directive":{"payload":{"renderingUpdates":[]}}}`,
		"missing resourceId":       `{"directive":{"payload":{"renderingUpdates":[{"resourceMetadata":{"payload":{}}}]}}}`,
		"empty resourceId":         `{"directive":{"payload":{"renderingUpdates":[{"resourceId":"","resourceMetadata":{"payload":{}}}]}}}`,
		"missing resourceMetadata": `{"directive":{"payload":{"renderingUpdates":[{"resourceId":"PUSH_VOLUME_CHANGE"}]}}}`,
		"missing payload":          `{"directive":{"payload":{"renderingUpdates":[{"resourceId":"PUSH_VOLUME_CHANGE","resourceMetadata":{}}]}}}`,
		"payload wrong type":       `{"directive":{"payload":{"renderingUpdates":[{"resourceId":"PUSH_VOLUME_CHANGE","resourceMetadata":{"payload":"not-an-object"}}]}}}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			evts, err := DecodeRenderingUpdate([]byte(body))
			require.NoError(t, err)
			assert.Empty(t, evts)
		})
	}
}

func TestDecodeRenderingUpdate_RejectsInvalidJSON(t *testing.T) {
	_, err := DecodeRenderingUpdate([]byte("{not json"))
	require.Error(t, err)
}

// TestDecodeRenderingUpdate_AllKnownTypes drives the decoder over every
// fixture in testdata/push/events/, asserting both the concrete event
// type and the wire enum value. This is the regression net for the
// big switch in buildTypedEvent — if anyone adds an enum value but
// forgets to wire up a case, this table will catch it (any fixture
// referencing the new wire string will fall through to
// UnknownPushEvent and fail an assertion).
func TestDecodeRenderingUpdate_AllKnownTypes(t *testing.T) {
	type expect struct {
		concrete any // pointer to a zero-value of the expected concrete type
		kind     PushMessageType
		serial   string
	}
	const dsn = "G090XXXXXX1234"
	cases := map[string]expect{
		"volume_change.json":          {concrete: (*VolumeChangeEvent)(nil), kind: PushVolumeChange, serial: dsn},
		"audio_player_state.json":     {concrete: (*AudioPlayerStateEvent)(nil), kind: PushAudioPlayerState, serial: dsn},
		"connection_status.json":      {concrete: (*ConnectionStatusEvent)(nil), kind: PushConnectionStatus, serial: dsn},
		"bluetooth_status.json":       {concrete: (*BluetoothStatusEvent)(nil), kind: PushBluetoothStatus, serial: dsn},
		"microphone_status.json":      {concrete: (*MicrophoneStatusEvent)(nil), kind: PushMicrophoneStatus, serial: dsn},
		"equalizer_state_change.json": {concrete: (*EqualizerStateChangeEvent)(nil), kind: PushEqualizerStateChange, serial: dsn},
		"media_queue_change.json":     {concrete: (*MediaQueueChangeEvent)(nil), kind: PushMediaQueueChange, serial: dsn},
		"media_change.json":           {concrete: (*MediaChangeEvent)(nil), kind: PushMediaChange, serial: dsn},
		"media_progress_change.json":  {concrete: (*MediaProgressChangeEvent)(nil), kind: PushMediaProgressChange, serial: dsn},
		"generic_activity.json":       {concrete: (*GenericActivityEvent)(nil), kind: PushGenericActivity, serial: dsn},
		"media_sessions_updated.json": {concrete: (*MediaSessionsUpdatedEvent)(nil), kind: PushMediaSessionsUpdated, serial: dsn},
		"now_playing_updated.json":    {concrete: (*NowPlayingUpdatedEvent)(nil), kind: PushNowPlayingUpdated, serial: dsn},
		"item_change.json":            {concrete: (*ItemChangeEvent)(nil), kind: PushItemChange, serial: dsn},
		"matter_device_found.json":    {concrete: (*MatterDeviceFoundEvent)(nil), kind: PushMatterDeviceFound, serial: ""},
	}
	for fixture, want := range cases {
		t.Run(fixture, func(t *testing.T) {
			raw := loadFixture(t, fixture)
			evts, err := DecodeRenderingUpdate(raw)
			require.NoError(t, err)
			require.Len(t, evts, 1)
			assert.IsType(t, want.concrete, evts[0])
			assert.Equal(t, want.kind, evts[0].Type())
			assert.Equal(t, want.serial, evts[0].SerialNumber())
		})
	}
}
