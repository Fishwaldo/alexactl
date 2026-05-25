package amazondevices

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Fishwaldo/goamazondevices/internal/amzconst"
)

func TestMedia_DeviceVolumes_HappyPath(t *testing.T) {
	body, err := os.ReadFile("testdata/media/device_volumes.json")
	require.NoError(t, err)
	ms, _ := newMediaTest(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, amzconst.URIDeviceVolumes, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})
	got, err := ms.DeviceVolumes(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 3, len(got))
	assert.Equal(t, VolumeState{Volume: 42, Muted: false}, *got["S-A"])
	assert.Equal(t, VolumeState{Volume: 0, Muted: true}, *got["S-B"])
}

func TestMedia_DeviceVolumes_HTTPErrorWraps(t *testing.T) {
	ms, _ := newMediaTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	_, err := ms.DeviceVolumes(context.Background())
	require.Error(t, err)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusForbidden, apiErr.StatusCode)
}

func TestMedia_MediaStates_HappyPath(t *testing.T) {
	body, err := os.ReadFile("testdata/media/media_states.json")
	require.NoError(t, err)
	ms, _ := newMediaTest(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, amzconst.URIMediaState, r.URL.Path)
		assert.Contains(t, r.URL.RawQuery, "deviceSerialNumber=S-A")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})
	devs := []*Device{
		{SerialNumber: "S-A", DeviceType: "T", MediaPlayerSupported: true},
		{SerialNumber: "S-B", DeviceType: "T", MediaPlayerSupported: true},
		{SerialNumber: "S-C", DeviceType: "T", MediaPlayerSupported: false},
	}
	got, err := ms.MediaStates(context.Background(), devs)
	require.NoError(t, err)
	require.Contains(t, got, "S-A")
	a := got["S-A"]
	assert.Equal(t, "PLAYING", a.PlayerState)
	assert.Equal(t, "Bohemian Rhapsody", a.NowPlayingTitle)
	assert.Equal(t, "Queen", a.NowPlayingLine1)
	assert.True(t, a.NextEnabled)
	assert.False(t, a.PreviousEnabled)
	assert.Equal(t, 354000*time.Millisecond, a.MediaLength)
	assert.Equal(t, 120500*time.Millisecond, a.MediaPosition)
	assert.Equal(t, "Amazon Music", a.MediaProvider)
	assert.Equal(t, "https://example/amzn.png", a.MediaProviderURL)
	assert.False(t, a.MediaPositionUpdated.IsZero())

	// S-B exists but has no progress/provider — fields zero-valued.
	require.Contains(t, got, "S-B")
	assert.Equal(t, "PAUSED", got["S-B"].PlayerState)
	assert.Equal(t, time.Duration(0), got["S-B"].MediaLength)

	// S-C is filtered out (MediaPlayerSupported=false).
	_, hasC := got["S-C"]
	assert.False(t, hasC)
}

func TestMedia_MediaStates_NoDevices(t *testing.T) {
	ms, _ := newMediaTest(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("must not call HTTP with empty device list")
	})
	got, err := ms.MediaStates(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestMedia_MediaStates_EmptySessionList(t *testing.T) {
	ms, _ := newMediaTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"mediaSessionList":[]}`))
	})
	devs := []*Device{{SerialNumber: "S", DeviceType: "T", MediaPlayerSupported: true}}
	got, err := ms.MediaStates(context.Background(), devs)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestMedia_MusicProviders_FiltersByPropertyAndAvailability(t *testing.T) {
	body, err := os.ReadFile("testdata/media/music_providers.json")
	require.NoError(t, err)
	ms, _ := newMediaTest(t, func(w http.ResponseWriter, r *http.Request) {
		// URIMusicProviders contains a query string; the path is the
		// leading "/api/behaviors/entities".
		assert.Contains(t, r.URL.Path, "/api/behaviors/entities")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})
	got, err := ms.MusicProviders(context.Background())
	require.NoError(t, err)

	// AVAILABLE + supports Alexa.Music.PlaySearchPhrase => surface.
	require.Contains(t, got, "amzn-music")
	assert.Equal(t, "Amazon Music", got["amzn-music"].ProviderName)
	assert.True(t, got["amzn-music"].DefaultProvider)
	require.Contains(t, got, "spotify")
	assert.False(t, got["spotify"].DefaultProvider)

	// UNAVAILABLE => dropped.
	_, hasT := got["tunein"]
	assert.False(t, hasT)
	// non-music => dropped.
	_, hasN := got["non-music-skill"]
	assert.False(t, hasN)
}
