package amazondevices

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Fishwaldo/goamazondevices/internal/amzconst"
	"github.com/Fishwaldo/goamazondevices/internal/httpwrapper"
)

func newMediaTest(t *testing.T, handler http.HandlerFunc) (*MediaService, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	hc := srv.Client()
	hc.Transport = &hostRewriter{base: srv.URL, inner: srv.Client().Transport}
	wr, err := httpwrapper.New(httpwrapper.Config{
		HTTPClient: hc,
		Site:       srv.URL,
		RandReader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 1024)),
	})
	require.NoError(t, err)
	sess := &Session{AccountCustomerID: "OWNER", Domain: "com"}
	// processBatch's debounce goroutine must NOT race the synchronous
	// Flush in facades that Send-N-then-Flush (fanoutAndFlush). A
	// no-op sleepFn lets the goroutine fire between Sends and POST a
	// partial batch concurrently with Flush's own POST, racing on the
	// shared `posted` map the handler decodes into. Blocking sleepFn
	// until cleanup guarantees Flush always wins; when the goroutine
	// finally wakes it sees the bumped batchGen and exits silently.
	releaseSleep := make(chan struct{})
	t.Cleanup(func() { close(releaseSleep) })
	seq := &SequenceService{
		wrapper: wr,
		session: sess,
		sleepFn: func(_ context.Context, _ time.Duration) error {
			<-releaseSleep
			return nil
		},
	}
	ms := &MediaService{
		wrapper:  wr,
		sequence: seq,
		session:  sess,
	}
	return ms, srv
}

func TestMedia_Speak_SinglePOST(t *testing.T) {
	var posted map[string]any
	ms, _ := newMediaTest(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, amzconst.URIBehaviorsPreview, r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&posted))
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	dev := &Device{
		SerialNumber:   "S-1",
		DeviceType:     "T-1",
		ClusterMembers: map[string]string{"S-1": "T-1"},
	}
	require.NoError(t, ms.Speak(context.Background(), dev, "hello world"))

	seq, _ := posted["sequenceJson"].(string)
	assert.Contains(t, seq, "Alexa.Speak")
	assert.Contains(t, seq, "hello world")
	assert.Contains(t, seq, "S-1")
}

func TestMedia_Announce_ClusterFanoutInTarget(t *testing.T) {
	var posted map[string]any
	ms, _ := newMediaTest(t, func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&posted))
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	dev := &Device{
		SerialNumber: "S-MAIN",
		DeviceType:   "T-MAIN",
		ClusterMembers: map[string]string{
			"S-LEFT":  "T-PAIR",
			"S-RIGHT": "T-PAIR",
		},
	}
	require.NoError(t, ms.Announce(context.Background(), dev, "dinner is ready"))
	seq, _ := posted["sequenceJson"].(string)
	assert.Contains(t, seq, "S-LEFT")
	assert.Contains(t, seq, "S-RIGHT")
	assert.Contains(t, seq, "dinner is ready")
}

func TestAnnounce_EmptyClusterFallback(t *testing.T) {
	var posted map[string]any
	ms, _ := newMediaTest(t, func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&posted))
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	dev := &Device{
		SerialNumber:   "S1",
		DeviceType:     "T1",
		ClusterMembers: nil,
	}
	require.NoError(t, ms.Announce(context.Background(), dev, "alone here"))

	seqJSON, _ := posted["sequenceJson"].(string)
	var seq map[string]any
	require.NoError(t, json.Unmarshal([]byte(seqJSON), &seq))

	// Drill into startNode.nodesToExecute[0].operationPayload.target.devices
	startNode, _ := seq["startNode"].(map[string]any)
	nodes, _ := startNode["nodesToExecute"].([]any)
	require.Len(t, nodes, 1)
	node, _ := nodes[0].(map[string]any)
	payload, _ := node["operationPayload"].(map[string]any)
	target, _ := payload["target"].(map[string]any)
	devices, _ := target["devices"].([]any)
	require.Len(t, devices, 1)
	entry, _ := devices[0].(map[string]any)
	assert.Equal(t, "S1", entry["deviceSerialNumber"])
	assert.Equal(t, "T1", entry["deviceTypeId"])
}

func TestMedia_SetVolume_PerClusterFanout(t *testing.T) {
	var posted map[string]any
	ms, _ := newMediaTest(t, func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&posted))
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	dev := &Device{
		SerialNumber: "S-MAIN",
		DeviceType:   "T-MAIN",
		ClusterMembers: map[string]string{
			"S-LEFT":  "T-PAIR",
			"S-RIGHT": "T-PAIR",
		},
	}
	require.NoError(t, ms.SetVolume(context.Background(), dev, 42))
	seq, _ := posted["sequenceJson"].(string)
	assert.Contains(t, seq, `"value":42`)
	assert.Equal(t, 2, strings.Count(seq, `"deviceSerialNumber":"S-LEFT"`)+strings.Count(seq, `"deviceSerialNumber":"S-RIGHT"`))
}

func TestMedia_SetVolume_RejectsOutOfRange(t *testing.T) {
	ms, _ := newMediaTest(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("must not reach the server")
	})
	dev := &Device{SerialNumber: "S", DeviceType: "T", ClusterMembers: map[string]string{"S": "T"}}
	assert.Error(t, ms.SetVolume(context.Background(), dev, -1))
	assert.Error(t, ms.SetVolume(context.Background(), dev, 101))
}
