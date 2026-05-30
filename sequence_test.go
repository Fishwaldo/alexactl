// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package alexactl

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Fishwaldo/alexactl/internal/amzconst"
	"github.com/Fishwaldo/alexactl/internal/cache"
	"github.com/Fishwaldo/alexactl/internal/httpwrapper"
)

func newSequenceTest(t *testing.T, handler http.HandlerFunc) (*SequenceService, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	hc := srv.Client()
	hc.Transport = &hostRewriter{base: srv.URL, inner: srv.Client().Transport}
	wr, err := httpwrapper.New(httpwrapper.Config{
		HTTPClient: hc,
		Site:       "https://www.amazon.com",
		RandReader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 1024)),
	})
	require.NoError(t, err)
	s := &SequenceService{
		wrapper: wr,
		session: &Session{AccountCustomerID: "OWNER", Domain: "com"},
		// no-op sleep so tests don't actually wait
		sleepFn: func(_ context.Context, _ time.Duration) error { return nil },
	}
	s.routinesCache = cache.NewCache[map[string]map[string]any](
		cache.Config{Name: "sequence.routines", TTL: time.Hour},
		func(ctx context.Context, _ string) (map[string]map[string]any, error) {
			return s.fetchRoutines(ctx)
		},
	)
	return s, srv
}

func TestSequence_buildOperationNode_Speak(t *testing.T) {
	s, _ := newSequenceTest(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("must not call HTTP for build-only test")
	})
	dev := &Device{SerialNumber: "S-1", DeviceType: "T-1"}
	node, err := s.buildOperationNode(context.Background(), dev, string(SeqSpeak), "hello", "")
	require.NoError(t, err)
	assert.Equal(t, amzconst.OperationNodeAttr, node["@type"])
	assert.Equal(t, string(SeqSpeak), node["type"])
	payload, _ := node["operationPayload"].(map[string]any)
	assert.Equal(t, "hello", payload["textToSpeak"])
	assert.Equal(t, "OWNER", payload["customerId"])
	assert.Equal(t, "S-1", payload["deviceSerialNumber"])
	target, _ := payload["target"].(map[string]any)
	devices, _ := target["devices"].([]any)
	require.Len(t, devices, 1)
	entry, _ := devices[0].(map[string]any)
	assert.Equal(t, "S-1", entry["deviceSerialNumber"])
	assert.Equal(t, "T-1", entry["deviceTypeId"])
	assert.Equal(t, "amzn1.ask.1p.saysomething", payload["skillId"])
}

func TestSequence_buildOperationNode_Announce_ClusterFanoutInTarget(t *testing.T) {
	s, _ := newSequenceTest(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("must not call HTTP for build-only test")
	})
	dev := &Device{
		SerialNumber: "S-MAIN",
		DeviceType:   "T-MAIN",
		ClusterMembers: map[string]string{
			"S-LEFT":  "T-PAIR",
			"S-RIGHT": "T-PAIR",
		},
	}
	node, err := s.buildOperationNode(context.Background(), dev, string(SeqAnnouncement), "dinner", "")
	require.NoError(t, err)
	payload, _ := node["operationPayload"].(map[string]any)
	assert.Equal(t, "PT5S", payload["expireAfter"])
	target, _ := payload["target"].(map[string]any)
	devices, _ := target["devices"].([]any)
	require.Len(t, devices, 2, "cluster fanout lives inside target.devices for Announce")
	assert.Equal(t, "amzn1.ask.1p.routines.messaging", payload["skillId"])
}

func TestSequence_buildOperationNode_VolumeStopSoundMusicTextSkill(t *testing.T) {
	s, _ := newSequenceTest(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("must not call HTTP for build-only test")
	})
	dev := &Device{SerialNumber: "S", DeviceType: "T"}

	cases := []struct {
		name       string
		kind       SequenceType
		body       any
		providerID string
		assertions func(t *testing.T, payload map[string]any)
	}{
		{"Volume", SeqVolume, 42, "", func(t *testing.T, p map[string]any) {
			assert.Equal(t, 42, p["value"])
		}},
		{"Stop", SeqStop, "", "", func(t *testing.T, p map[string]any) {
			devices, _ := p["devices"].([]any)
			require.Len(t, devices, 1)
			assert.Equal(t, "amzn1.ask.1p.alexadevicecontrols", p["skillId"])
		}},
		{"Sound", SeqSound, "alexa_sfx_bell", "", func(t *testing.T, p map[string]any) {
			assert.Equal(t, "alexa_sfx_bell", p["soundStringId"])
			assert.Equal(t, "amzn1.ask.1p.sound", p["skillId"])
		}},
		{"Music", SeqMusic, "Bohemian Rhapsody", "amazon-music", func(t *testing.T, p map[string]any) {
			assert.Equal(t, "Bohemian Rhapsody", p["searchPhrase"])
			assert.Equal(t, "Bohemian Rhapsody", p["sanitizedSearchPhrase"])
			assert.Equal(t, "amazon-music", p["musicProviderId"])
		}},
		{"TextCommand", SeqTextCommand, "what time is it", "", func(t *testing.T, p map[string]any) {
			assert.Equal(t, "what time is it", p["text"])
			assert.Equal(t, "amzn1.ask.1p.tellalexa", p["skillId"])
		}},
		{"LaunchSkill", SeqLaunchSkill, "MySkill", "", func(t *testing.T, p map[string]any) {
			cr, _ := p["connectionRequest"].(map[string]any)
			assert.Equal(t, "connection://AMAZON.Launch/MySkill", cr["uri"])
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			node, err := s.buildOperationNode(context.Background(), dev, string(c.kind), c.body, c.providerID)
			require.NoError(t, err)
			payload, _ := node["operationPayload"].(map[string]any)
			c.assertions(t, payload)
		})
	}
}

func TestSequence_buildOperationNode_InfoSkill(t *testing.T) {
	s, _ := newSequenceTest(t, func(_ http.ResponseWriter, _ *http.Request) {})
	dev := &Device{SerialNumber: "S", DeviceType: "T"}
	node, err := s.buildOperationNode(context.Background(), dev, "Alexa.Weather.Play", "", "")
	require.NoError(t, err)
	payload, _ := node["operationPayload"].(map[string]any)
	// Info-skill bodies are the base payload only — no message-specific keys.
	assert.Equal(t, "OWNER", payload["customerId"])
	_, hasText := payload["textToSpeak"]
	assert.False(t, hasText)
}

func TestSequence_buildOperationNode_Unknown(t *testing.T) {
	s, _ := newSequenceTest(t, func(_ http.ResponseWriter, _ *http.Request) {})
	dev := &Device{SerialNumber: "S", DeviceType: "T"}
	_, err := s.buildOperationNode(context.Background(), dev, "Alexa.NotAThing", "", "")
	require.Error(t, err)
}

func TestSequence_buildOperationNode_NoSession(t *testing.T) {
	s, _ := newSequenceTest(t, func(_ http.ResponseWriter, _ *http.Request) {})
	s.session = nil
	dev := &Device{SerialNumber: "S", DeviceType: "T"}
	_, err := s.buildOperationNode(context.Background(), dev, string(SeqSpeak), "hi", "")
	require.ErrorIs(t, err, ErrCannotAuthenticate)
}

func TestSequence_postSequences_HappyPath(t *testing.T) {
	var posted map[string]any
	s, _ := newSequenceTest(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, amzconst.URIBehaviorsPreview, r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&posted))
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	op := sequenceOp{
		messageType:   string(SeqSpeak),
		operationNode: map[string]any{"@type": amzconst.OperationNodeAttr, "type": string(SeqSpeak), "operationPayload": map[string]any{"x": 1}},
		serial:        "S",
	}
	require.NoError(t, s.postSequences(context.Background(), []sequenceOp{op}))

	assert.Equal(t, "PREVIEW", posted["behaviorId"])
	assert.Equal(t, "ENABLED", posted["status"])
	seqJSON, _ := posted["sequenceJson"].(string)
	var seq map[string]any
	require.NoError(t, json.Unmarshal([]byte(seqJSON), &seq))
	assert.Equal(t, amzconst.SequenceTypeAttr, seq["@type"])
	start, _ := seq["startNode"].(map[string]any)
	assert.Equal(t, amzconst.SerialNodeAttr, start["@type"])
	nodes, _ := start["nodesToExecute"].([]any)
	require.Len(t, nodes, 1)
}

func TestSequence_postSequences_HTTPErrorWraps(t *testing.T) {
	s, _ := newSequenceTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	op := sequenceOp{operationNode: map[string]any{"type": string(SeqSpeak)}}
	err := s.postSequences(context.Background(), []sequenceOp{op})
	require.Error(t, err)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusForbidden, apiErr.StatusCode)
}

func TestSequence_postSequences_EmptyIsNoop(t *testing.T) {
	s, _ := newSequenceTest(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("must not call HTTP for empty batch")
	})
	require.NoError(t, s.postSequences(context.Background(), nil))
}

func TestSequence_Send_BuildsAndBuffers(t *testing.T) {
	var posts atomic.Int32
	s, _ := newSequenceTest(t, func(w http.ResponseWriter, _ *http.Request) {
		posts.Add(1)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	// Capture the sleep so we can release the goroutine on demand.
	release := make(chan struct{})
	s.sleepFn = func(_ context.Context, _ time.Duration) error {
		<-release
		return nil
	}

	dev := &Device{SerialNumber: "S", DeviceType: "T"}
	require.NoError(t, s.Send(context.Background(), dev, SeqSpeak, "hello", ""))
	// Send returns before the POST happens.
	assert.Equal(t, int32(0), posts.Load())

	// Release the timer; the goroutine drains the buffer and POSTs.
	close(release)

	// Spin briefly for the goroutine to finish. Two seconds is generous
	// for an in-process httptest call.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if posts.Load() == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	assert.Equal(t, int32(1), posts.Load(), "processBatch must POST exactly once after sleep")
}

func TestSequence_Send_SecondCallSharesBatch(t *testing.T) {
	var mu sync.Mutex
	var bodies []map[string]any
	s, _ := newSequenceTest(t, func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&b))
		mu.Lock()
		bodies = append(bodies, b)
		mu.Unlock()
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	release := make(chan struct{})
	s.sleepFn = func(_ context.Context, _ time.Duration) error {
		<-release
		return nil
	}

	devA := &Device{SerialNumber: "A", DeviceType: "T"}
	devB := &Device{SerialNumber: "B", DeviceType: "T"}
	require.NoError(t, s.Send(context.Background(), devA, SeqSpeak, "hi A", ""))
	require.NoError(t, s.Send(context.Background(), devB, SeqSpeak, "hi B", ""))
	close(release)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(bodies)
		mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, bodies, 1, "must batch into ONE POST")
	seqJSON, _ := bodies[0]["sequenceJson"].(string)
	assert.Contains(t, seqJSON, `"deviceSerialNumber":"A"`)
	assert.Contains(t, seqJSON, `"deviceSerialNumber":"B"`)
}

func TestSequence_Flush_BeatsTimer(t *testing.T) {
	var bodies []map[string]any
	var bodiesMu sync.Mutex
	s, _ := newSequenceTest(t, func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&b))
		bodiesMu.Lock()
		bodies = append(bodies, b)
		bodiesMu.Unlock()
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	// Sleep blocks forever — Flush must win.
	s.sleepFn = func(ctx context.Context, _ time.Duration) error {
		<-ctx.Done()
		return ctx.Err()
	}

	dev := &Device{SerialNumber: "S", DeviceType: "T"}
	require.NoError(t, s.Send(context.Background(), dev, SeqSpeak, "hi", ""))
	require.NoError(t, s.Flush(context.Background()))
	bodiesMu.Lock()
	defer bodiesMu.Unlock()
	require.Len(t, bodies, 1)
}

func TestSequence_Flush_EmptyIsNoop(t *testing.T) {
	s, _ := newSequenceTest(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("must not call HTTP for empty flush")
	})
	require.NoError(t, s.Flush(context.Background()))
}

func TestSequence_Flush_PropagatesHTTPError(t *testing.T) {
	s, _ := newSequenceTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	s.sleepFn = func(ctx context.Context, _ time.Duration) error {
		<-ctx.Done()
		return ctx.Err()
	}
	dev := &Device{SerialNumber: "S", DeviceType: "T"}
	require.NoError(t, s.Send(context.Background(), dev, SeqSpeak, "hi", ""))
	err := s.Flush(context.Background())
	require.Error(t, err)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusForbidden, apiErr.StatusCode)
}

func TestSequence_Optimise_SameOpDifferentDevicesParallelWraps(t *testing.T) {
	s, _ := newSequenceTest(t, func(_ http.ResponseWriter, _ *http.Request) {})
	ops := []sequenceOp{
		{messageType: string(SeqSpeak), messageBody: "hi", serial: "A",
			operationNode: map[string]any{"type": string(SeqSpeak), "serial": "A"}},
		{messageType: string(SeqSpeak), messageBody: "hi", serial: "B",
			operationNode: map[string]any{"type": string(SeqSpeak), "serial": "B"}},
		{messageType: string(SeqSpeak), messageBody: "hi", serial: "C",
			operationNode: map[string]any{"type": string(SeqSpeak), "serial": "C"}},
	}
	nodes := s.optimiseSequenceNodes(ops)
	require.Len(t, nodes, 1)
	parent, _ := nodes[0].(map[string]any)
	assert.Equal(t, amzconst.ParallelNodeAttr, parent["@type"])
	children, _ := parent["nodesToExecute"].([]any)
	assert.Len(t, children, 3)
}

func TestSequence_Optimise_DifferentOpsStayInline(t *testing.T) {
	s, _ := newSequenceTest(t, func(_ http.ResponseWriter, _ *http.Request) {})
	ops := []sequenceOp{
		{messageType: string(SeqSpeak), messageBody: "hi", serial: "A",
			operationNode: map[string]any{"type": string(SeqSpeak)}},
		{messageType: string(SeqVolume), messageBody: 42, serial: "A",
			operationNode: map[string]any{"type": string(SeqVolume)}},
	}
	nodes := s.optimiseSequenceNodes(ops)
	require.Len(t, nodes, 2)
	for _, n := range nodes {
		m, _ := n.(map[string]any)
		assert.NotEqual(t, amzconst.ParallelNodeAttr, m["@type"])
	}
}

func TestSequence_Optimise_SameOpSameDeviceDoesNotWrap(t *testing.T) {
	s, _ := newSequenceTest(t, func(_ http.ResponseWriter, _ *http.Request) {})
	ops := []sequenceOp{
		{messageType: string(SeqSpeak), messageBody: "hi", serial: "A",
			operationNode: map[string]any{"type": string(SeqSpeak), "i": 1}},
		{messageType: string(SeqSpeak), messageBody: "hi", serial: "A",
			operationNode: map[string]any{"type": string(SeqSpeak), "i": 2}},
	}
	nodes := s.optimiseSequenceNodes(ops)
	require.Len(t, nodes, 2, "same op on same device must not parallel-wrap")
}

func TestSequence_Optimise_MusicGroupedByProvider(t *testing.T) {
	s, _ := newSequenceTest(t, func(_ http.ResponseWriter, _ *http.Request) {})
	ops := []sequenceOp{
		{messageType: string(SeqMusic), messageBody: "song1", providerID: "amzn", serial: "A",
			operationNode: map[string]any{}},
		{messageType: string(SeqMusic), messageBody: "song1", providerID: "spot", serial: "B",
			operationNode: map[string]any{}},
	}
	nodes := s.optimiseSequenceNodes(ops)
	// Different providerIDs => different groups => NO parallel wrap.
	require.Len(t, nodes, 2)
}

func TestSequence_Routines_FetchesAndCachesEnabled(t *testing.T) {
	body, err := os.ReadFile("testdata/sequence/automations.json")
	require.NoError(t, err)
	var callsMu sync.Mutex
	var calls int
	s, _ := newSequenceTest(t, func(w http.ResponseWriter, r *http.Request) {
		callsMu.Lock()
		calls++
		callsMu.Unlock()
		assert.Equal(t, amzconst.URIAutomations, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})

	names, err := s.Routines(context.Background())
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"Good Morning", "Bedtime"}, names)
	callsMu.Lock()
	assert.Equal(t, 1, calls)
	callsMu.Unlock()

	// Second call hits cache.
	names2, err := s.Routines(context.Background())
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"Good Morning", "Bedtime"}, names2)
	callsMu.Lock()
	assert.Equal(t, 1, calls, "Routines must cache between calls")
	callsMu.Unlock()
}

func TestSequence_RefreshRoutines_BustsCache(t *testing.T) {
	body, err := os.ReadFile("testdata/sequence/automations.json")
	require.NoError(t, err)
	var callsMu sync.Mutex
	var calls int
	s, _ := newSequenceTest(t, func(w http.ResponseWriter, _ *http.Request) {
		callsMu.Lock()
		calls++
		callsMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})

	_, err = s.Routines(context.Background())
	require.NoError(t, err)
	require.NoError(t, s.RefreshRoutines(context.Background()))
	_, err = s.Routines(context.Background())
	require.NoError(t, err)
	callsMu.Lock()
	assert.Equal(t, 2, calls, "RefreshRoutines must trigger a fresh fetch")
	callsMu.Unlock()
}

func TestSequence_Routines_HTTPErrorWraps(t *testing.T) {
	s, _ := newSequenceTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	_, err := s.Routines(context.Background())
	require.Error(t, err)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusForbidden, apiErr.StatusCode)
}

func TestSequence_buildOperationNode_Routines(t *testing.T) {
	body, err := os.ReadFile("testdata/sequence/automations.json")
	require.NoError(t, err)
	s, _ := newSequenceTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})
	// Prime the cache.
	_, err = s.Routines(context.Background())
	require.NoError(t, err)

	dev := &Device{SerialNumber: "REAL-S", DeviceType: "REAL-T"}
	node, err := s.buildOperationNode(context.Background(), dev, string(SeqRoutine), "Good Morning", "")
	require.NoError(t, err)

	// The returned node IS the routine's startNode with placeholders
	// substituted.
	payload, _ := node["operationPayload"].(map[string]any)
	assert.Equal(t, "REAL-S", payload["deviceSerialNumber"])
	assert.Equal(t, "REAL-T", payload["deviceType"])
	assert.Equal(t, "Good morning!", payload["textToSpeak"])
}

func TestSequence_buildOperationNode_Routines_Unknown(t *testing.T) {
	body, err := os.ReadFile("testdata/sequence/automations.json")
	require.NoError(t, err)
	s, _ := newSequenceTest(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	})
	_, err = s.Routines(context.Background())
	require.NoError(t, err)

	dev := &Device{SerialNumber: "S", DeviceType: "T"}
	_, err = s.buildOperationNode(context.Background(), dev, string(SeqRoutine), "Nonexistent", "")
	require.Error(t, err)
}

func TestSequence_replaceRoutinePlaceholders_NestedSubstitution(t *testing.T) {
	dev := &Device{SerialNumber: "S", DeviceType: "T"}
	in := map[string]any{
		"x": "ALEXA_CURRENT_DSN",
		"y": "ALEXA_CURRENT_DEVICE_TYPE",
		"z": []any{
			map[string]any{"a": "ALEXA_CURRENT_DSN"},
			"plain-string",
		},
		"keep": "irrelevant",
	}
	out := replaceRoutinePlaceholders(in, dev)
	assert.Equal(t, "S", out["x"])
	assert.Equal(t, "T", out["y"])
	zs, _ := out["z"].([]any)
	require.Len(t, zs, 2)
	first, _ := zs[0].(map[string]any)
	assert.Equal(t, "S", first["a"])
	assert.Equal(t, "plain-string", zs[1])
	assert.Equal(t, "irrelevant", out["keep"])
}
