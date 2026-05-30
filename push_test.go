// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package alexactl

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultSubscribeConfig(t *testing.T) {
	c := defaultSubscribeConfig()
	assert.Equal(t, 16, c.chanCapacity)
	assert.False(t, c.initialSyncOnFirstEvent)
}

func TestWithPushChannelCapacity_AppliesValue(t *testing.T) {
	c := defaultSubscribeConfig()
	WithPushChannelCapacity(64).apply(c)
	assert.Equal(t, 64, c.chanCapacity)
}

func TestWithPushChannelCapacity_ClampsTooSmall(t *testing.T) {
	c := defaultSubscribeConfig()
	WithPushChannelCapacity(0).apply(c)
	assert.Equal(t, 1, c.chanCapacity, "0 should clamp to 1")

	WithPushChannelCapacity(-5).apply(c)
	assert.Equal(t, 1, c.chanCapacity, "negative should clamp to 1")
}

func TestWithInitialSyncOnFirstEvent_SetsFlag(t *testing.T) {
	c := defaultSubscribeConfig()
	WithInitialSyncOnFirstEvent().apply(c)
	assert.True(t, c.initialSyncOnFirstEvent)
}

func TestSubscribeOptions_LaterWins(t *testing.T) {
	c := defaultSubscribeConfig()
	WithPushChannelCapacity(8).apply(c)
	WithPushChannelCapacity(32).apply(c)
	assert.Equal(t, 32, c.chanCapacity)
}

func newPushTestService(t *testing.T) *PushService {
	t.Helper()
	return &PushService{
		streamClient: http.DefaultClient,
	}
}

func TestRegisterCapabilities_204SetsFlag(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		require := func(cond bool, msg string) {
			if !cond {
				t.Fatal(msg)
			}
		}
		require(r.Method == http.MethodPut, "expected PUT")
		require(strings.HasSuffix(r.URL.Path, "/v1/devices/@self/capabilities"), "wrong path: "+r.URL.Path)
		require(r.Header.Get("Authorization") == "Bearer atok", "missing/wrong bearer")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	ps := newPushTestService(t)
	ps.streamClient = srv.Client()
	ps.capabilitiesURLFn = func() string { return srv.URL + "/v1/devices/@self/capabilities" }
	ps.setSession(&Session{})

	if err := ps.registerCapabilities(context.Background(), "atok"); err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if !ps.capabilitiesRegistered.Load() {
		t.Fatal("expected in-process flag to be set after 204")
	}
	if !ps.session().CapabilitiesRegistered {
		t.Fatal("expected session.CapabilitiesRegistered to be true")
	}

	// Calling again should be a no-op (no second PUT).
	if err := ps.registerCapabilities(context.Background(), "atok"); err != nil {
		t.Fatalf("second call: expected nil err, got %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("expected exactly 1 PUT, got %d", hits.Load())
	}
}

func TestRegisterCapabilities_NonNoContentReturnsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ps := newPushTestService(t)
	ps.streamClient = srv.Client()
	ps.capabilitiesURLFn = func() string { return srv.URL + "/v1/devices/@self/capabilities" }
	ps.setSession(&Session{})

	err := ps.registerCapabilities(context.Background(), "atok")
	if err == nil {
		t.Fatal("expected error on 500, got nil")
	}
	if !errors.Is(err, ErrCannotRegisterDevice) {
		t.Fatalf("expected ErrCannotRegisterDevice, got %v", err)
	}
	if ps.capabilitiesRegistered.Load() {
		t.Fatal("flag must remain false on failure")
	}
}

// Compile-time check: PushService satisfies sessionConsumer (it
// implements setSession). Future tasks add Push to Auth.consumers; this
// assertion locks the contract now so a future signature change
// doesn't silently break the broadcast wiring.
var _ sessionConsumer = (*PushService)(nil)
