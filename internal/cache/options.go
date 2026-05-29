package cache

import "time"

// CacheClass categorises caches by expected freshness. Per-class TTL
// is configurable; defaults below.
//
//nolint:revive // CacheClass + ClassRealtime avoids stutter better than Class + Realtime
type CacheClass int

const (
	// ClassRealtime — live device state (volumes, media). Default 15s.
	ClassRealtime CacheClass = iota
	// ClassShort — schedules, lists, DnD. Default 5m.
	ClassShort
	// ClassLong — catalogues, routines, devices. Default 1h.
	ClassLong
)

// ClassConfig maps a CacheClass to a TTL duration. Zero disables a
// class entirely (caches in that class always miss to the loader).
type ClassConfig struct {
	ttls map[CacheClass]time.Duration
}

// DefaultClassConfig returns the package-default TTLs.
func DefaultClassConfig() *ClassConfig {
	return &ClassConfig{
		ttls: map[CacheClass]time.Duration{
			ClassRealtime: 15 * time.Second,
			ClassShort:    5 * time.Minute,
			ClassLong:     time.Hour,
		},
	}
}

// Set overrides the TTL for a class. d <= 0 disables the class.
func (c *ClassConfig) Set(class CacheClass, d time.Duration) {
	if d < 0 {
		d = 0
	}
	c.ttls[class] = d
}

// TTL returns the configured duration for class. Unknown classes
// return zero.
func (c *ClassConfig) TTL(class CacheClass) time.Duration {
	return c.ttls[class]
}
