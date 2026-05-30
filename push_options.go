// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package alexactl

// SubscribeOption configures a single Subscribe call. Options are
// applied left-to-right; later options win on conflict.
type SubscribeOption interface {
	apply(*subscribeConfig)
}

// subscribeConfig is the resolved per-subscriber configuration. Zero
// value mirrors the defaults — chanCapacity 16, no initial sync.
type subscribeConfig struct {
	// chanCapacity is the buffered size of the channel returned to
	// the caller. Drop-oldest backpressure kicks in when the buffer
	// fills.
	chanCapacity int

	// initialSyncOnFirstEvent triggers a one-shot synthetic emission
	// of *VolumeChangeEvent per known device the first time the
	// underlying stream delivers any event to this subscriber.
	initialSyncOnFirstEvent bool
}

// defaultSubscribeConfig returns the spec-documented defaults.
func defaultSubscribeConfig() *subscribeConfig {
	return &subscribeConfig{chanCapacity: 16}
}

type subscribeOptionFunc func(*subscribeConfig)

func (f subscribeOptionFunc) apply(c *subscribeConfig) { f(c) }

// WithPushChannelCapacity overrides the per-subscriber channel buffer.
// Values < 1 are silently clamped to 1 (an unbuffered push channel
// would back-pressure into the shared upstream stream — refused).
func WithPushChannelCapacity(n int) SubscribeOption {
	return subscribeOptionFunc(func(c *subscribeConfig) {
		if n < 1 {
			n = 1
		}
		c.chanCapacity = n
	})
}

// WithInitialSyncOnFirstEvent requests a one-shot synthetic burst of
// *VolumeChangeEvent for each device known to the Devices service,
// delivered to this subscriber's channel the first time the stream
// emits any event. The burst lets callers reconstruct current volume
// state without waiting for users to interact with each device.
//
// History.Vocal sync is documented in the spec but not wired in this
// release — History is available without exposing Vocal-grouped
// records.
func WithInitialSyncOnFirstEvent() SubscribeOption {
	return subscribeOptionFunc(func(c *subscribeConfig) {
		c.initialSyncOnFirstEvent = true
	})
}
