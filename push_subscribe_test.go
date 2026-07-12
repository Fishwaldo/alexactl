// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package alexactl

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pushTestRig wires a PushService against an httptest server that
// serves both /capabilities (PUT 204) and /directives (multipart
// stream). Use newPushTestRig to start it; t.Cleanup tears it down.
type pushTestRig struct {
	srv        *httptest.Server
	mux        *http.ServeMux
	ps         *PushService
	directives chan func(http.ResponseWriter) // each func writes one part
}

func newPushTestRig(t *testing.T) *pushTestRig {
	t.Helper()
	mux := http.NewServeMux()
	rig := &pushTestRig{
		mux:        mux,
		directives: make(chan func(http.ResponseWriter), 16),
	}
	mux.HandleFunc("/v1/devices/@self/capabilities", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/directives", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "multipart/related; boundary=B")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		for {
			select {
			case <-r.Context().Done():
				return
			case fn, ok := <-rig.directives:
				if !ok {
					_, _ = fmt.Fprint(w, "--B--\r\n")
					if flusher != nil {
						flusher.Flush()
					}
					return
				}
				fn(w)
				if flusher != nil {
					flusher.Flush()
				}
			}
		}
	})
	srv := httptest.NewServer(mux)
	rig.srv = srv

	rig.ps = &PushService{streamClient: srv.Client()}
	rig.ps.setSession(&Session{AccessToken: "atok"})
	rig.ps.capabilitiesURLFn = func() string { return srv.URL + "/v1/devices/@self/capabilities" }
	rig.ps.streamURLFn = func() string { return srv.URL + "/directives" }

	t.Cleanup(func() {
		srv.Close()
	})
	return rig
}

func (r *pushTestRig) sendPart(body string) {
	r.directives <- func(w http.ResponseWriter) {
		_, _ = fmt.Fprintf(w, "--B\r\nContent-Type: application/json\r\n\r\n%s\r\n", body)
	}
}

func (r *pushTestRig) closeStream() { close(r.directives) }

const volumeChangePart = `{"directive":{"payload":{"renderingUpdates":[{"resourceId":"PUSH_VOLUME_CHANGE","resourceMetadata":{"payload":{"dopplerId":{"deviceSerialNumber":"S1","deviceType":"T"},"volumeSetting":33,"isMuted":false}}}]}}}`

func TestPush_Subscribe_HappyPath(t *testing.T) {
	rig := newPushTestRig(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := rig.ps.Subscribe(ctx)
	require.NoError(t, err)

	rig.sendPart(volumeChangePart)
	// Close the stream so the multipart parser sees the closing
	// boundary and surfaces the buffered part body to the decoder.
	rig.closeStream()

	select {
	case evt := <-ch:
		v, ok := evt.(*VolumeChangeEvent)
		require.True(t, ok, "expected *VolumeChangeEvent, got %T", evt)
		assert.Equal(t, "S1", v.SerialNumber())
		assert.Equal(t, 33, v.Volume)
	case <-time.After(2 * time.Second):
		t.Fatal("no event delivered within 2s")
	}

	cancel()
	// channel closes on subscriber-ctx cancel
	select {
	case _, open := <-ch:
		assert.False(t, open, "channel must close after ctx cancel")
	case <-time.After(2 * time.Second):
		t.Fatal("subscriber channel did not close")
	}
}

func TestPush_Subscribe_RejectsBeforeSession(t *testing.T) {
	ps := &PushService{streamClient: http.DefaultClient}
	_, err := ps.Subscribe(context.Background())
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "Push.Subscribe before Auth.Login/Restore"))
}

func TestPush_Subscribe_FanOutToMultipleSubscribers(t *testing.T) {
	rig := newPushTestRig(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	chA, err := rig.ps.Subscribe(ctx)
	require.NoError(t, err)
	chB, err := rig.ps.Subscribe(ctx)
	require.NoError(t, err)
	chC, err := rig.ps.Subscribe(ctx)
	require.NoError(t, err)

	rig.sendPart(volumeChangePart)
	rig.closeStream() // flush the multipart parser (next boundary unblocks the read)

	for _, ch := range []<-chan PushEvent{chA, chB, chC} {
		select {
		case evt := <-ch:
			v, ok := evt.(*VolumeChangeEvent)
			require.True(t, ok)
			assert.Equal(t, "S1", v.SerialNumber())
		case <-time.After(2 * time.Second):
			t.Fatal("subscriber did not receive event")
		}
	}
}

func TestPush_Subscribe_PerSubscriberContextCancellationOnly(t *testing.T) {
	rig := newPushTestRig(t)

	ctxA, cancelA := context.WithCancel(context.Background())
	defer cancelA()
	ctxB, cancelB := context.WithCancel(context.Background())
	defer cancelB()

	chA, err := rig.ps.Subscribe(ctxA)
	require.NoError(t, err)
	chB, err := rig.ps.Subscribe(ctxB)
	require.NoError(t, err)

	// Send 1st part + 2nd part to flush the 1st through the parser.
	rig.sendPart(volumeChangePart)
	rig.sendPart(volumeChangePart)

	// Drain the 1st event on both subscribers.
	for _, ch := range []<-chan PushEvent{chA, chB} {
		select {
		case <-ch:
		case <-time.After(2 * time.Second):
			t.Fatal("expected first event on both subscribers")
		}
	}

	// Cancel A. Its channel must close.
	cancelA()
	select {
	case _, open := <-chA:
		assert.False(t, open, "chA must close after ctxA cancel")
	case <-time.After(2 * time.Second):
		t.Fatal("chA did not close after ctxA cancel")
	}

	// Send 3rd part (flushes the 2nd through the parser).
	// chB should receive the 2nd event; chA is closed and gets nothing.
	rig.sendPart(volumeChangePart)
	select {
	case evt := <-chB:
		_, ok := evt.(*VolumeChangeEvent)
		assert.True(t, ok, "chB should still receive after chA unsubscribed")
	case <-time.After(2 * time.Second):
		t.Fatal("chB did not receive after chA unsubscribed")
	}
}

// newReconnectingRig serves /directives that closes the body after one
// part, then a second body that delivers another part. The injected
// sleepFn captures the requested delay so the test can assert backoff
// scheduling without actually sleeping.
type reconnectingRig struct {
	*pushTestRig
	openCount  *atomic.Int32
	sleepCalls chan time.Duration
}

func newReconnectingRig(t *testing.T) *reconnectingRig {
	t.Helper()
	mux := http.NewServeMux()
	rig := &reconnectingRig{
		pushTestRig: &pushTestRig{mux: mux, directives: make(chan func(http.ResponseWriter), 16)},
		openCount:   new(atomic.Int32),
		sleepCalls:  make(chan time.Duration, 16),
	}
	mux.HandleFunc("/v1/devices/@self/capabilities", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/directives", func(w http.ResponseWriter, r *http.Request) {
		n := rig.openCount.Add(1)
		w.Header().Set("Content-Type", "multipart/related; boundary=B")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		fmt.Fprintf(w, "--B\r\nContent-Type: application/json\r\n\r\n%s\r\n", volumeChangePart)
		if flusher != nil {
			flusher.Flush()
		}
		// First connection: bail without closing boundary, so the
		// client sees an EOF mid-stream and reconnects.
		if n == 1 {
			return
		}
		// Second connection: send a clean closing boundary and block
		// on ctx.Done so the streamLoop can observe the cleanup.
		fmt.Fprint(w, "--B--\r\n")
		if flusher != nil {
			flusher.Flush()
			<-r.Context().Done()
		}
	})
	srv := httptest.NewServer(mux)
	rig.srv = srv

	rig.ps = &PushService{streamClient: srv.Client()}
	rig.ps.setSession(&Session{AccessToken: "atok"})
	rig.ps.capabilitiesURLFn = func() string { return srv.URL + "/v1/devices/@self/capabilities" }
	rig.ps.streamURLFn = func() string { return srv.URL + "/directives" }
	rig.ps.sleepFn = func(_ context.Context, d time.Duration) error {
		rig.sleepCalls <- d
		return nil // skip the actual sleep
	}
	t.Cleanup(srv.Close)
	return rig
}

func TestPush_Subscribe_ReconnectsAfterDisconnect(t *testing.T) {
	rig := newReconnectingRig(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := rig.ps.Subscribe(ctx)
	require.NoError(t, err)

	// First open delivers one event, then disconnects.
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("no event from first connection")
	}
	// Second open delivers a second event.
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("no event after reconnect")
	}
	assert.GreaterOrEqual(t, rig.openCount.Load(), int32(2))
}

func TestPush_Backoff_StartsAtMinAndDoubles(t *testing.T) {
	// Spy sleepFn captures delays; we never actually wait. The
	// inverse: capabilities registration always fails so streamLoop
	// keeps re-tripping the backoff path.
	var sleeps []time.Duration
	var sleepsMu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError) // capabilities fail
	}))
	defer srv.Close()

	ps := &PushService{streamClient: srv.Client()}
	ps.setSession(&Session{AccessToken: "atok"})
	ps.capabilitiesURLFn = func() string { return srv.URL + "/" }
	ps.streamURLFn = func() string { return srv.URL + "/" }

	want := []time.Duration{
		5 * time.Second, 10 * time.Second, 20 * time.Second, 40 * time.Second,
	}
	doneCh := make(chan struct{})
	ps.sleepFn = func(_ context.Context, d time.Duration) error {
		sleepsMu.Lock()
		sleeps = append(sleeps, d)
		done := len(sleeps) >= len(want)
		sleepsMu.Unlock()
		if done {
			select {
			case <-doneCh:
			default:
				close(doneCh)
			}
		}
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := ps.Subscribe(ctx)
	require.NoError(t, err)

	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("backoff loop never recorded enough sleeps")
	}

	sleepsMu.Lock()
	got := sleeps[:len(want)]
	sleepsMu.Unlock()
	assert.Equal(t, want, got)
}

func TestPush_Subscribe_Reauth_DeliversEventThenCloses(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/devices/@self/capabilities", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/directives", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ps := &PushService{streamClient: srv.Client()}
	ps.setSession(&Session{AccessToken: "atok"})
	ps.capabilitiesURLFn = func() string { return srv.URL + "/v1/devices/@self/capabilities" }
	ps.streamURLFn = func() string { return srv.URL + "/directives" }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := ps.Subscribe(ctx)
	require.NoError(t, err)

	// First (and only) event is *ReauthRequiredEvent.
	select {
	case evt := <-ch:
		_, ok := evt.(*ReauthRequiredEvent)
		require.True(t, ok, "expected *ReauthRequiredEvent, got %T", evt)
	case <-time.After(2 * time.Second):
		t.Fatal("no event delivered for reauth")
	}
	// Channel must then close.
	select {
	case _, open := <-ch:
		assert.False(t, open, "subscriber channel must close after reauth event")
	case <-time.After(2 * time.Second):
		t.Fatal("subscriber channel did not close after reauth")
	}
}

func TestPush_Subscribe_BackpressureDropsOldest(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/devices/@self/capabilities", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	startSend := make(chan struct{})
	mux.HandleFunc("/directives", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "multipart/related; boundary=B")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		<-startSend
		// Send 20 parts back-to-back — far more than the consumer
		// buffer of 1.
		for i := 0; i < 20; i++ {
			body := fmt.Sprintf(`{"directive":{"payload":{"renderingUpdates":[{"resourceId":"PUSH_VOLUME_CHANGE","resourceMetadata":{"payload":{"dopplerId":{"deviceSerialNumber":"S1","deviceType":"T"},"volumeSetting":%d,"isMuted":false}}}]}}}`, i)
			fmt.Fprintf(w, "--B\r\nContent-Type: application/json\r\n\r\n%s\r\n", body)
			if flusher != nil {
				flusher.Flush()
			}
		}
		<-r.Context().Done()
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ps := &PushService{streamClient: srv.Client()}
	ps.setSession(&Session{AccessToken: "atok"})
	ps.capabilitiesURLFn = func() string { return srv.URL + "/v1/devices/@self/capabilities" }
	ps.streamURLFn = func() string { return srv.URL + "/directives" }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := ps.Subscribe(ctx, WithPushChannelCapacity(1))
	require.NoError(t, err)

	close(startSend)

	// Sleep long enough for the server to push all 20 parts and the
	// drop-oldest loop to repeatedly evict. Then drain — we should
	// see fewer than 20 events.
	time.Sleep(200 * time.Millisecond)

	var collected []*VolumeChangeEvent
drain:
	for {
		select {
		case evt, ok := <-ch:
			if !ok {
				break drain
			}
			collected = append(collected, evt.(*VolumeChangeEvent))
		case <-time.After(100 * time.Millisecond):
			break drain
		}
	}
	assert.Less(t, len(collected), 20,
		"backpressure must drop events; got %d (expected <20)", len(collected))
	assert.GreaterOrEqual(t, len(collected), 1,
		"at least one event should make it through")
}

func TestPush_PingLoop_FiresAfterConnect(t *testing.T) {
	var pings atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/devices/@self/capabilities", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/ping", func(w http.ResponseWriter, _ *http.Request) {
		pings.Add(1)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/directives", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "multipart/related; boundary=B")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done()
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ps := &PushService{streamClient: srv.Client()}
	ps.setSession(&Session{AccessToken: "atok"})
	ps.capabilitiesURLFn = func() string { return srv.URL + "/v1/devices/@self/capabilities" }
	ps.streamURLFn = func() string { return srv.URL + "/directives" }
	ps.pingURLFn = func() string { return srv.URL + "/ping" }

	pingsObserved := make(chan struct{}, 4)
	ps.sleepFn = func(ctx context.Context, _ time.Duration) error {
		// Fire immediately instead of waiting 280s.
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		pingsObserved <- struct{}{}
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := ps.Subscribe(ctx)
	require.NoError(t, err)

	// Wait for at least one ping.
	select {
	case <-pingsObserved:
	case <-time.After(2 * time.Second):
		t.Fatal("ping loop never invoked sleep")
	}
	// Give the ping POST a moment to land.
	for i := 0; i < 50 && pings.Load() == 0; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	assert.GreaterOrEqual(t, pings.Load(), int32(1))
}

// TestPush_PingLoop_NonReauthErrorClosesConnAndReconnects ports upstream
// #899: a ping failure that is NOT reauth-class (e.g. a transient 500)
// must close the stream so streamLoop reconnects, instead of looping
// forever on a half-open connection. The /directives handler never
// EOFs or closes on its own — the only way a second open happens is if
// pingLoop closes the first conn after the /ping 500.
func TestPush_PingLoop_NonReauthErrorClosesConnAndReconnects(t *testing.T) {
	var opens atomic.Int32
	var pings atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/devices/@self/capabilities", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/ping", func(w http.ResponseWriter, _ *http.Request) {
		pings.Add(1)
		w.WriteHeader(http.StatusInternalServerError) // non-reauth failure
	})
	mux.HandleFunc("/directives", func(w http.ResponseWriter, r *http.Request) {
		opens.Add(1)
		w.Header().Set("Content-Type", "multipart/related; boundary=B")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// Half-open: no more data, no EOF, no close — only the client
		// (via a fixed pingLoop closing conn) ends this request.
		<-r.Context().Done()
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ps := &PushService{streamClient: srv.Client()}
	ps.setSession(&Session{AccessToken: "atok"})
	ps.capabilitiesURLFn = func() string { return srv.URL + "/v1/devices/@self/capabilities" }
	ps.streamURLFn = func() string { return srv.URL + "/directives" }
	ps.pingURLFn = func() string { return srv.URL + "/ping" }
	ps.sleepFn = func(ctx context.Context, _ time.Duration) error {
		// Fire immediately instead of waiting pingInterval/backoff —
		// used both by pingLoop's interval sleep and streamLoop's
		// backoffSleep.
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := ps.Subscribe(ctx)
	require.NoError(t, err)

	require.Eventually(t, func() bool { return opens.Load() >= 2 }, 3*time.Second, 10*time.Millisecond,
		"streamLoop never reconnected after a non-reauth ping failure — "+
			"pingLoop must close the conn instead of looping on a dead connection")
	assert.GreaterOrEqual(t, pings.Load(), int32(1))
}

func TestPush_Subscribe_InitialSyncOnFirstEvent(t *testing.T) {
	rig := newPushTestRig(t)
	rig.ps.initialSyncFn = func(_ context.Context) (map[string]*VolumeState, error) {
		return map[string]*VolumeState{
			"DEV-A": {Volume: 11, Muted: false},
			"DEV-B": {Volume: 22, Muted: true},
		}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := rig.ps.Subscribe(ctx, WithInitialSyncOnFirstEvent())
	require.NoError(t, err)

	rig.sendPart(volumeChangePart) // first live event triggers sync
	rig.sendPart(volumeChangePart) // flush the first part through the parser

	// Collect events for a bounded period.
	seen := map[string]int{}
	deadline := time.After(2 * time.Second)
loop:
	for {
		select {
		case evt := <-ch:
			v := evt.(*VolumeChangeEvent)
			seen[v.SerialNumber()] = v.Volume
			if len(seen) >= 3 {
				break loop
			}
		case <-deadline:
			break loop
		}
	}
	assert.Equal(t, 33, seen["S1"], "live event must be delivered")
	assert.Equal(t, 11, seen["DEV-A"], "synthetic initial-sync event for DEV-A")
	assert.Equal(t, 22, seen["DEV-B"], "synthetic initial-sync event for DEV-B")
}
