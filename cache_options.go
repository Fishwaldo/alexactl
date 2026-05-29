package amazondevices

import (
	"time"

	"github.com/Fishwaldo/goamazondevices/internal/cache"
)

// CacheClass re-exports cache.CacheClass so callers don't import the
// internal package directly.
type CacheClass = cache.CacheClass

const (
	// ClassRealtime — live device state. Default TTL 15s.
	ClassRealtime = cache.ClassRealtime
	// ClassShort — schedules, lists, DnD. Default TTL 5m.
	ClassShort = cache.ClassShort
	// ClassLong — catalogues, devices. Default TTL 1h.
	ClassLong = cache.ClassLong
)

// Persistence re-exports cache.Persistence so importing apps can write
// adapters without depending on the internal package.
type Persistence = cache.Persistence

// NamedSnapshot re-exports cache.NamedSnapshot for adapter authors.
type NamedSnapshot = cache.NamedSnapshot

// FilePersistence re-exports cache.NewFilePersistence as the common
// single-instance-daemon adapter.
func FilePersistence(path string) Persistence {
	return cache.NewFilePersistence(path)
}

// WithCacheTTL overrides the TTL for one CacheClass. d <= 0 disables
// caching for that class entirely.
func WithCacheTTL(class CacheClass, d time.Duration) Option {
	return optionFunc(func(cfg *config) {
		if cfg.cacheTTLOverrides == nil {
			cfg.cacheTTLOverrides = map[CacheClass]time.Duration{}
		}
		cfg.cacheTTLOverrides[class] = d
	})
}

// DisableCache is sugar for WithCacheTTL(class, 0).
func DisableCache(class CacheClass) Option {
	return WithCacheTTL(class, 0)
}

// WithCachePersistence wires a save/restore adapter. The library calls
// Persistence.Load once during NewClient and Persistence.Save on every
// Client.SnapshotCaches call. Errors from the adapter are logged at
// WARN and do not abort cache operations.
func WithCachePersistence(p Persistence) Option {
	return optionFunc(func(cfg *config) { cfg.cachePersistence = p })
}

// WithPushCacheBridge enables push-event-driven cache invalidation.
// When set, NewClient subscribes to PushService and routes events to
// matching cache invalidations via an internal table.
func WithPushCacheBridge() Option {
	return optionFunc(func(cfg *config) { cfg.pushCacheBridge = true })
}
