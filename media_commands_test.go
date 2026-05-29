package amazondevices

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMedia_InfoSkills_ReturnsCopy(t *testing.T) {
	ms, _ := newMediaTest(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("must not call HTTP for list-only query")
	})
	a := ms.InfoSkills()
	require.NotEmpty(t, a)
	assert.Contains(t, a, "Alexa.Weather.Play")
	a[0] = "MUTATED"
	b := ms.InfoSkills()
	assert.NotEqual(t, "MUTATED", b[0])
}

func TestMedia_PlaySound_PerClusterFanout(t *testing.T) {
	var posted map[string]any
	ms, _ := newMediaTest(t, func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&posted))
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	ms.validateSoundFn = func(_ context.Context, _ string) error { return nil }

	dev := &Device{
		SerialNumber: "S-MAIN", DeviceType: "T-MAIN",
		ClusterMembers: map[string]string{"S-LEFT": "T-PAIR", "S-RIGHT": "T-PAIR"},
	}
	require.NoError(t, ms.PlaySound(context.Background(), dev, "alexa_sfx_bell"))
	seqJSON, _ := posted["sequenceJson"].(string)
	assert.Contains(t, seqJSON, `"soundStringId":"alexa_sfx_bell"`)
	assert.Contains(t, seqJSON, "ParallelNode")
}

func TestMedia_TextCommand_SinglePOST(t *testing.T) {
	var posted map[string]any
	ms, _ := newMediaTest(t, func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&posted))
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	dev := &Device{SerialNumber: "S", DeviceType: "T",
		ClusterMembers: map[string]string{"S": "T"}}
	require.NoError(t, ms.TextCommand(context.Background(), dev, "what time is it"))
	seqJSON, _ := posted["sequenceJson"].(string)
	assert.Contains(t, seqJSON, `"text":"what time is it"`)
	assert.Equal(t, 1, strings.Count(seqJSON, `"type":"Alexa.TextCommand"`))
}

func TestMedia_LaunchSkill(t *testing.T) {
	var posted map[string]any
	ms, _ := newMediaTest(t, func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&posted))
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	dev := &Device{SerialNumber: "S", DeviceType: "T",
		ClusterMembers: map[string]string{"S": "T"}}
	require.NoError(t, ms.LaunchSkill(context.Background(), dev, "MySkill"))
	seqJSON, _ := posted["sequenceJson"].(string)
	assert.Contains(t, seqJSON, `connection://AMAZON.Launch/MySkill`)
}

func TestMedia_PlayMusic_OK(t *testing.T) {
	var posted map[string]any
	ms, _ := newMediaTest(t, func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&posted))
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	// Skip the live MusicProviders lookup in this unit test.
	ms.validateMusicProviderFn = func(_ context.Context, providerID string) error {
		if providerID == "amazon-music" {
			return nil
		}
		return assert.AnError
	}
	dev := &Device{SerialNumber: "S", DeviceType: "T",
		ClusterMembers: map[string]string{"S": "T"}}
	require.NoError(t, ms.PlayMusic(context.Background(), dev, "Bohemian Rhapsody", "amazon-music"))
	seqJSON, _ := posted["sequenceJson"].(string)
	assert.Contains(t, seqJSON, `"musicProviderId":"amazon-music"`)
	assert.Contains(t, seqJSON, `"searchPhrase":"Bohemian Rhapsody"`)
}

func TestMedia_PlayMusic_UnknownProviderRejected(t *testing.T) {
	ms, _ := newMediaTest(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("must not reach the server when provider is invalid")
	})
	ms.validateMusicProviderFn = func(_ context.Context, _ string) error {
		return assert.AnError
	}
	dev := &Device{SerialNumber: "S", DeviceType: "T",
		ClusterMembers: map[string]string{"S": "T"}}
	err := ms.PlayMusic(context.Background(), dev, "song", "nonexistent")
	require.Error(t, err)
}

func TestMedia_Control_StopGoesThroughSequence(t *testing.T) {
	var posted map[string]any
	ms, _ := newMediaTest(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/behaviors/preview", r.URL.Path,
			"Stop must route through the Sequence pipeline")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&posted))
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	dev := &Device{SerialNumber: "S", DeviceType: "T",
		ClusterMembers: map[string]string{"S": "T"}}
	require.NoError(t, ms.Control(context.Background(), dev, CmdStop))
	seqJSON, _ := posted["sequenceJson"].(string)
	assert.Contains(t, seqJSON, "Alexa.DeviceControls.Stop")
}

func TestMedia_Control_PlayGoesThroughDirectPOST(t *testing.T) {
	var callsMu sync.Mutex
	var calls []string
	var posted map[string]any
	ms, _ := newMediaTest(t, func(w http.ResponseWriter, r *http.Request) {
		callsMu.Lock()
		calls = append(calls, r.URL.Path+"?"+r.URL.RawQuery)
		callsMu.Unlock()
		require.NoError(t, json.NewDecoder(r.Body).Decode(&posted))
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	dev := &Device{SerialNumber: "S", DeviceType: "T",
		ClusterMembers: map[string]string{"S": "T"}}
	require.NoError(t, ms.Control(context.Background(), dev, CmdPlay))
	callsMu.Lock()
	defer callsMu.Unlock()
	require.Len(t, calls, 1)
	assert.Contains(t, calls[0], "/api/np/command")
	assert.Contains(t, calls[0], "deviceSerialNumber=S")
	assert.Contains(t, calls[0], "deviceType=T")
	assert.Equal(t, "PlayCommand", posted["type"])
}

func TestMedia_Control_DirectPOSTPropagatesHTTPError(t *testing.T) {
	ms, _ := newMediaTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	dev := &Device{SerialNumber: "S", DeviceType: "T",
		ClusterMembers: map[string]string{"S": "T"}}
	err := ms.Control(context.Background(), dev, CmdNext)
	require.Error(t, err)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusForbidden, apiErr.StatusCode)
}

func TestMedia_CallRoutine_ExpandsRoutinePayload(t *testing.T) {
	automations, err := os.ReadFile("testdata/sequence/automations.json")
	require.NoError(t, err)
	var posted map[string]any
	ms, _ := newMediaTest(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/behaviors/v2/automations":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(automations)
		case "/api/behaviors/preview":
			require.NoError(t, json.NewDecoder(r.Body).Decode(&posted))
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	})
	dev := &Device{SerialNumber: "REAL-S", DeviceType: "REAL-T",
		ClusterMembers: map[string]string{"REAL-S": "REAL-T"}}
	require.NoError(t, ms.CallRoutine(context.Background(), dev, "Good Morning"))

	seqJSON, _ := posted["sequenceJson"].(string)
	assert.Contains(t, seqJSON, `"deviceSerialNumber":"REAL-S"`)
	assert.Contains(t, seqJSON, `"textToSpeak":"Good morning!"`)
}

func TestMedia_CallRoutine_UnknownNameRejected(t *testing.T) {
	automations, err := os.ReadFile("testdata/sequence/automations.json")
	require.NoError(t, err)
	ms, _ := newMediaTest(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/behaviors/v2/automations", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(automations)
	})
	dev := &Device{SerialNumber: "S", DeviceType: "T",
		ClusterMembers: map[string]string{"S": "T"}}
	err = ms.CallRoutine(context.Background(), dev, "NonexistentRoutine")
	require.Error(t, err)
}

func TestMedia_InfoSkill_HappyPath(t *testing.T) {
	var posted map[string]any
	ms, _ := newMediaTest(t, func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&posted))
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	dev := &Device{SerialNumber: "S", DeviceType: "T",
		ClusterMembers: map[string]string{"S": "T"}}
	require.NoError(t, ms.InfoSkill(context.Background(), dev, "Alexa.Weather.Play"))
	seqJSON, _ := posted["sequenceJson"].(string)
	assert.Contains(t, seqJSON, `"type":"Alexa.Weather.Play"`)
}

func TestMedia_InfoSkill_UnknownRejected(t *testing.T) {
	ms, _ := newMediaTest(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("must not reach the server")
	})
	dev := &Device{SerialNumber: "S", DeviceType: "T"}
	err := ms.InfoSkill(context.Background(), dev, "Alexa.NotARealInfoSkill")
	require.Error(t, err)
}

func TestMedia_PlaySound_RejectsUnknownSound(t *testing.T) {
	ms, _ := newMediaTest(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("must not reach the server for an unknown sound")
	})
	ms.validateSoundFn = func(_ context.Context, soundID string) error {
		return fmt.Errorf("amazondevices: sound %q not available", soundID)
	}

	dev := &Device{SerialNumber: "S", DeviceType: "T",
		ClusterMembers: map[string]string{"S": "T"}}
	err := ms.PlaySound(context.Background(), dev, "ghost_sound")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `sound "ghost_sound" not available`)
}

func TestMedia_PlaySound_AcceptsCatalogSound(t *testing.T) {
	body, err := os.ReadFile("testdata/media/sounds.json")
	require.NoError(t, err)

	var posted map[string]any
	ms, _ := newMediaTest(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/api/behaviors/entities"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		default:
			require.NoError(t, json.NewDecoder(r.Body).Decode(&posted))
			_, _ = w.Write([]byte(`{"ok":true}`))
		}
	})
	dev := &Device{SerialNumber: "S", DeviceType: "T",
		ClusterMembers: map[string]string{"S": "T"}}
	require.NoError(t,
		ms.PlaySound(context.Background(), dev, "amzn_sfx_doorbell_01"))
	seqJSON, _ := posted["sequenceJson"].(string)
	assert.Contains(t, seqJSON, `"soundStringId":"amzn_sfx_doorbell_01"`)
}
