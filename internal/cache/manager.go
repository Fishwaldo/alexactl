// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package cache

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Manager owns the set of caches in a Client. Responsible for tag
// invalidation and (in Task 7) snapshot/restore fan-out.
type Manager struct {
	logger  *slog.Logger
	classes *ClassConfig
	persist Persistence

	mu     sync.RWMutex
	caches []registered
}

// registered is the type-erased view a Manager keeps of each Cache.
// The closures capture the cache's concrete V type so the Manager
// itself remains non-generic.
type registered struct {
	name     string
	tags     []string
	clearAll func()
	snapshot func() ([]byte, error)
	restore  func([]byte) error
}

// NewManager constructs a Manager. logger and classes are mandatory;
// persist may be nil (no save/restore).
func NewManager(logger *slog.Logger, classes *ClassConfig, persist Persistence) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	if classes == nil {
		classes = DefaultClassConfig()
	}
	return &Manager{
		logger:  logger,
		classes: classes,
		persist: persist,
	}
}

// Classes returns the configured TTL config.
func (m *Manager) Classes() *ClassConfig { return m.classes }

// Persistence returns the configured adapter (may be nil).
func (m *Manager) Persistence() Persistence { return m.persist }

// Len returns the number of registered caches.
func (m *Manager) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.caches)
}

// Register binds a cache to the manager. marshal and unmarshal are
// optional: pass nil to opt out of persistence for this cache.
func Register[V any](m *Manager, c *Cache[V],
	marshal func(map[string]Entry[V]) ([]byte, error),
	unmarshal func([]byte) (map[string]Entry[V], error)) {

	snap := func() ([]byte, error) {
		if marshal == nil {
			return nil, nil
		}
		c.mu.RLock()
		cp := make(map[string]Entry[V], len(c.entries))
		for k, v := range c.entries {
			cp[k] = v
		}
		c.mu.RUnlock()
		return marshal(cp)
	}
	rest := func(b []byte) error {
		if unmarshal == nil {
			return nil
		}
		loaded, err := unmarshal(b)
		if err != nil {
			return err
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		now := timeNow()
		c.entries = make(map[string]Entry[V], len(loaded))
		for k, e := range loaded {
			if !e.ExpiresAt.IsZero() && now.After(e.ExpiresAt) {
				continue
			}
			c.entries[k] = e
		}
		return nil
	}
	m.mu.Lock()
	m.caches = append(m.caches, registered{
		name:     c.cfg.Name,
		tags:     append([]string(nil), c.cfg.Tags...),
		clearAll: c.InvalidateAll,
		snapshot: snap,
		restore:  rest,
	})
	m.mu.Unlock()
}

// InvalidateTag clears every cache carrying tag.
func (m *Manager) InvalidateTag(tag string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, r := range m.caches {
		for _, t := range r.tags {
			if t == tag {
				r.clearAll()
				break
			}
		}
	}
}

// timeNow is a package-level seam so tests can pin time without
// reaching into every cache. Production uses time.Now.
var timeNow = time.Now

// Restore loads snapshots via the configured Persistence and feeds
// each into the matching registered cache. Persistence errors are
// logged at WARN and Restore returns without populating caches.
// Per-cache unmarshal errors are logged at WARN; other caches still
// restore.
func (m *Manager) Restore(ctx context.Context) error {
	if m.persist == nil {
		return nil
	}
	snaps, err := m.persist.Load(ctx)
	if err != nil {
		m.logger.WarnContext(ctx, "cache: load failed; starting with empty caches",
			"err", err)
		return err
	}
	byName := map[string][]byte{}
	for _, s := range snaps {
		byName[s.Name] = s.Bytes
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, r := range m.caches {
		raw, ok := byName[r.name]
		if !ok {
			continue
		}
		if r.restore == nil {
			continue
		}
		if rerr := r.restore(raw); rerr != nil {
			m.logger.WarnContext(ctx, "cache: restore failed; cache starts empty",
				"name", r.name, "err", rerr)
			continue
		}
	}
	return nil
}

// Snapshot serializes every registered cache (skipping any that opted
// out by passing nil marshal at Register time) and hands the slice to
// the configured Persistence adapter. Per-cache marshal errors are
// logged at WARN and that cache is skipped; other caches still persist.
func (m *Manager) Snapshot(ctx context.Context) error {
	if m.persist == nil {
		return nil
	}
	m.mu.RLock()
	out := make([]NamedSnapshot, 0, len(m.caches))
	for _, r := range m.caches {
		if r.snapshot == nil {
			continue
		}
		bytes, err := r.snapshot()
		if err != nil {
			m.logger.WarnContext(ctx, "cache: marshal failed; skipping",
				"name", r.name, "err", err)
			continue
		}
		if bytes == nil {
			continue
		}
		out = append(out, NamedSnapshot{
			Name:    r.name,
			Bytes:   bytes,
			SavedAt: timeNow(),
		})
	}
	m.mu.RUnlock()
	return m.persist.Save(ctx, out)
}
