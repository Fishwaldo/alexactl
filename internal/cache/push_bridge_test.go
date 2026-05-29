package cache

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeEvent struct {
	kind   string
	serial string
}

func (f fakeEvent) Kind() string         { return f.kind }
func (f fakeEvent) SerialNumber() string { return f.serial }

func TestBridge_VolumeChangeInvalidatesByKey(t *testing.T) {
	m := NewManager(slog.Default(), DefaultClassConfig(), nil)
	volumes := NewCache[string](Config{Name: "media.deviceVolumes", TTL: time.Hour}, nil)
	Register(m, volumes, nil, nil)
	volumes.Set("S1", "v1")
	volumes.Set("S2", "v2")

	ch := make(chan PushEventLike, 1)
	b := NewPushBridge(m, BridgeTargets{
		DeviceVolumes: volumes,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go b.Run(ctx, ch)

	ch <- fakeEvent{kind: "VolumeChange", serial: "S1"}

	require.Eventually(t, func() bool {
		_, hit, _ := volumes.Get(context.Background(), "S1")
		return !hit
	}, time.Second, 5*time.Millisecond)

	_, hit2, _ := volumes.Get(context.Background(), "S2")
	assert.True(t, hit2, "S2 should remain")
	close(ch)
}

func TestBridge_ReauthInvalidatesSessionTag(t *testing.T) {
	m := NewManager(slog.Default(), DefaultClassConfig(), nil)
	tagged := NewCache[string](Config{Name: "media.musicProviders", TTL: time.Hour, Tags: []string{"session"}}, nil)
	plain := NewCache[string](Config{Name: "plain", TTL: time.Hour}, nil)
	Register(m, tagged, nil, nil)
	Register(m, plain, nil, nil)
	tagged.Set("k", "v")
	plain.Set("k", "v")

	ch := make(chan PushEventLike, 1)
	b := NewPushBridge(m, BridgeTargets{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go b.Run(ctx, ch)

	ch <- fakeEvent{kind: "ReauthRequired"}

	require.Eventually(t, func() bool {
		_, hit, _ := tagged.Get(context.Background(), "k")
		return !hit
	}, time.Second, 5*time.Millisecond)

	_, plainHit, _ := plain.Get(context.Background(), "k")
	assert.True(t, plainHit, "untagged cache should NOT be cleared")
	close(ch)
}

func TestBridge_UnmappedEventIsNoop(t *testing.T) {
	m := NewManager(slog.Default(), DefaultClassConfig(), nil)
	volumes := NewCache[string](Config{Name: "media.deviceVolumes", TTL: time.Hour}, nil)
	Register(m, volumes, nil, nil)
	volumes.Set("S1", "v1")

	ch := make(chan PushEventLike, 1)
	b := NewPushBridge(m, BridgeTargets{DeviceVolumes: volumes})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go b.Run(ctx, ch)

	ch <- fakeEvent{kind: "UnknownKind"}
	time.Sleep(30 * time.Millisecond)
	_, hit, _ := volumes.Get(context.Background(), "S1")
	assert.True(t, hit, "unmapped event should leave cache intact")
	close(ch)
}

func TestBridge_NotificationChangeFiresClosure(t *testing.T) {
	m := NewManager(slog.Default(), DefaultClassConfig(), nil)
	var called atomic.Bool
	ch := make(chan PushEventLike, 1)
	b := NewPushBridge(m, BridgeTargets{
		Closures: BridgeClosures{
			NotificationsInvalidateAll: func() { called.Store(true) },
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go b.Run(ctx, ch)

	ch <- fakeEvent{kind: "NotificationChange"}
	require.Eventually(t, called.Load, time.Second, 5*time.Millisecond)
	close(ch)
}
