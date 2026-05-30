// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package alexactl

import (
	"context"

	"github.com/Fishwaldo/alexactl/internal/cache"
)

// pushEventAdapter wraps a root-package PushEvent so it satisfies
// cache.PushEventLike without leaking the root type into the internal
// package.
type pushEventAdapter struct{ evt PushEvent }

// Kind extracts the wire kind string from the PushMessageType.
func (a pushEventAdapter) Kind() string { return string(a.evt.Type()) }

// SerialNumber implements cache.PushEventLike.
func (a pushEventAdapter) SerialNumber() string { return a.evt.SerialNumber() }

// startPushCacheBridge subscribes to PushService and forwards every
// event through the adapter into a cache.PushBridge. The bridge
// goroutine exits when the subscription channel closes (Client
// teardown via PushService.stopStreamLoop).
//
// Called once per Client lifetime, gated by c.pushBridgeOnce.
func (c *Client) startPushCacheBridge() {
	sub, err := c.Push.Subscribe(context.Background())
	if err != nil {
		c.wrapper.Logger(context.Background()).WarnContext(context.Background(),
			"alexactl: failed to start push cache bridge",
			"err", err)
		return
	}
	targets := cache.BridgeTargets{
		DeviceVolumes: nil, // placeholder until Cache[string] mapping lands
		Closures: cache.BridgeClosures{
			MediaStatesInvalidateAll:    c.Media.mediaStatesCache.InvalidateAll,
			NotificationsInvalidateAll:  c.schedulesCache.InvalidateAll,
			ListsInvalidateAll:          c.Lists.itemsCache.InvalidateAll,
			MusicProvidersInvalidateAll: c.Media.musicProvidersCache.InvalidateAll,
			DevicesListInvalidateAll:    c.Devices.listCache.InvalidateAll,
			// Endpoints folded into devices.list (the loader fetches both),
			// so the endpoints-targeted events just clear the combined cache.
			EndpointsInvalidateAll: c.Devices.listCache.InvalidateAll,
		},
	}
	// DeviceVolumes is Cache[map[string]*VolumeState] not Cache[string],
	// so its closure routes through the generic InvalidateAll for now.
	// The bridge's typed VolumeChange handler operates on Cache[string]
	// (per-key invalidation). We expose a wrapper that invalidates the
	// whole volume cache instead — simpler and a small win.
	volumesInvalidate := c.Media.deviceVolumesCache.InvalidateAll
	bridge := cache.NewPushBridge(c.cacheManager, targets)
	adapted := make(chan cache.PushEventLike, 16)
	go func() {
		defer close(adapted)
		for evt := range sub {
			// Volume changes invalidate the per-account cache directly here
			// (the bridge's typed DeviceVolumes is wired in a later phase).
			if evt.Type() == "VolumeChange" {
				volumesInvalidate()
			}
			adapted <- pushEventAdapter{evt: evt}
		}
	}()
	go bridge.Run(context.Background(), adapted)
}
