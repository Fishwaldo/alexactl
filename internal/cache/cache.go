// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

// Package cache provides a generic, in-memory cache with class-based
// TTLs, single-flight loader semantics, tag invalidation, and a
// persistence-adapter contract for save/restore between runs.
//
// Sealed under internal/ — not part of the v1.0.0 public API surface.
package cache

import (
	"context"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// Config configures a single Cache instance at construction time.
type Config struct {
	// Name is the stable persistence key. Unique within a Manager.
	Name string
	// TTL is the entry lifetime. Zero or negative disables caching:
	// every Get with a loader runs the loader; Set still stores but
	// every subsequent Get sees the entry as expired.
	TTL time.Duration
	// Tags are invalidation tags. A Manager.InvalidateTag(tag) call
	// clears every cache carrying tag.
	Tags []string
}

// Entry is the on-the-shelf representation of a cached value. Exported
// so cross-package marshal/unmarshal closures (for persistence) can
// name the type when registering caches via Register.
type Entry[V any] struct {
	Value     V         `json:"value"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Cache is a single named cache keyed by string holding V values.
type Cache[V any] struct {
	cfg    Config
	loader func(ctx context.Context, key string) (V, error)

	mu      sync.RWMutex
	entries map[string]Entry[V]
	sf      singleflight.Group
}

// NewCache constructs a Cache. loader is optional; when nil, Get on a
// miss returns (zero, false, nil) without calling out.
func NewCache[V any](cfg Config, loader func(context.Context, string) (V, error)) *Cache[V] {
	return &Cache[V]{
		cfg:     cfg,
		loader:  loader,
		entries: map[string]Entry[V]{},
	}
}

// Name returns the cache's configured name (persistence key).
func (c *Cache[V]) Name() string { return c.cfg.Name }

// Tags returns a copy of the configured invalidation tags.
func (c *Cache[V]) Tags() []string {
	out := make([]string, len(c.cfg.Tags))
	copy(out, c.cfg.Tags)
	return out
}

// Set stores value under key with the cache's configured TTL.
func (c *Cache[V]) Set(key string, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var exp time.Time
	if c.cfg.TTL > 0 {
		exp = time.Now().Add(c.cfg.TTL)
	}
	c.entries[key] = Entry[V]{
		Value:     value,
		ExpiresAt: exp,
	}
}

// Get returns the cached value. With a loader, miss or expired triggers
// a single-flight loader call; concurrent misses for the same key share
// the flight. Loader errors are returned verbatim and the entry is NOT
// cached on failure.
func (c *Cache[V]) Get(ctx context.Context, key string) (V, bool, error) {
	var zero V

	if v, ok := c.lookup(key); ok {
		return v, true, nil
	}
	if c.loader == nil {
		return zero, false, nil
	}

	val, err, _ := c.sf.Do(key, func() (any, error) {
		// Second lookup inside the flight — another caller may have
		// populated the entry between our miss and our acquisition of
		// the singleflight slot.
		if v, ok := c.lookup(key); ok {
			return v, nil
		}
		v, lerr := c.loader(ctx, key)
		if lerr != nil {
			return zero, lerr
		}
		c.Set(key, v)
		return v, nil
	})
	if err != nil {
		return zero, false, err
	}
	return val.(V), true, nil
}

// Invalidate removes one entry.
func (c *Cache[V]) Invalidate(key string) {
	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

// InvalidateAll removes every entry.
func (c *Cache[V]) InvalidateAll() {
	c.mu.Lock()
	c.entries = map[string]Entry[V]{}
	c.mu.Unlock()
}

// lookup is the read-locked read-and-check-expiry helper.
func (c *Cache[V]) lookup(key string) (V, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var zero V
	e, ok := c.entries[key]
	if !ok {
		return zero, false
	}
	if e.ExpiresAt.IsZero() || time.Now().After(e.ExpiresAt) {
		return zero, false
	}
	return e.Value, true
}
