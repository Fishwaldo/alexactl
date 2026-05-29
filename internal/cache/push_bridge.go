package cache

import (
	"context"
)

// PushEventLike is the subset of the root-package PushEvent interface
// the bridge consumes. Defined locally so internal/cache does NOT
// depend on the root package.
type PushEventLike interface {
	// Kind returns a short string identifier for the event family —
	// matches the wire PushMessageType ("VolumeChange",
	// "NotificationChange", "ItemChange", etc.).
	Kind() string
	SerialNumber() string
}

// BridgeTargets enumerates the typed and closure-bag caches the bridge
// can invalidate. nil fields/closures are tolerated; the bridge skips
// routes whose target is unset.
type BridgeTargets struct {
	// DeviceVolumes is the only typed pointer because VolumeChangeEvent
	// carries a SerialNumber that the bridge invalidates per-key.
	DeviceVolumes *Cache[string]

	// Closures lets each remaining cache expose only InvalidateAll
	// without leaking its concrete V type into BridgeTargets.
	Closures BridgeClosures
}

// BridgeClosures collects per-cache InvalidateAll closures used by the
// bridge's static routing table.
type BridgeClosures struct {
	MediaStatesInvalidateAll    func()
	NotificationsInvalidateAll  func()
	ListsInvalidateAll          func()
	DevicesListInvalidateAll    func()
	EndpointsInvalidateAll      func()
	MusicProvidersInvalidateAll func()
}

// PushBridge routes PushEvents to cache invalidations using a static
// table. Construct with NewPushBridge; drive with Run.
type PushBridge struct {
	manager *Manager
	targets BridgeTargets
}

// NewPushBridge constructs a PushBridge bound to manager.
func NewPushBridge(manager *Manager, targets BridgeTargets) *PushBridge {
	return &PushBridge{manager: manager, targets: targets}
}

// Run consumes events from ch until ch closes or ctx is cancelled.
// One event is processed per loop iteration; routing is O(1).
func (b *PushBridge) Run(ctx context.Context, ch <-chan PushEventLike) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-ch:
			if !ok {
				return
			}
			b.route(evt)
		}
	}
}

func (b *PushBridge) route(evt PushEventLike) {
	switch evt.Kind() {
	case "VolumeChange":
		if b.targets.DeviceVolumes != nil {
			if evt.SerialNumber() != "" {
				b.targets.DeviceVolumes.Invalidate(evt.SerialNumber())
			} else {
				b.targets.DeviceVolumes.InvalidateAll()
			}
		}
	case "NotificationChange":
		if f := b.targets.Closures.NotificationsInvalidateAll; f != nil {
			f()
		}
	case "ItemChange":
		if f := b.targets.Closures.ListsInvalidateAll; f != nil {
			f()
		}
	case "MediaChange", "NowPlayingUpdated", "MediaSessionsUpdated",
		"MediaProgressChange", "AudioPlayerState", "MediaQueueChange":
		if f := b.targets.Closures.MediaStatesInvalidateAll; f != nil {
			f()
		}
	case "ConnectionStatus", "BluetoothStatus", "MicrophoneStatus", "EqualizerStateChange":
		if f := b.targets.Closures.DevicesListInvalidateAll; f != nil {
			f()
		}
	case "MatterDeviceFound":
		if f := b.targets.Closures.DevicesListInvalidateAll; f != nil {
			f()
		}
		if f := b.targets.Closures.EndpointsInvalidateAll; f != nil {
			f()
		}
	case "ReauthRequired":
		b.manager.InvalidateTag("session")
	default:
		// no-op
	}
}
