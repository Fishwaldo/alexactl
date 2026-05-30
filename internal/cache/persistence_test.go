// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package cache

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryPersistence_RoundTrip(t *testing.T) {
	p := NewMemoryPersistence()
	snaps := []NamedSnapshot{
		{Name: "a", Bytes: []byte("payload-a"), SavedAt: time.Now()},
		{Name: "b", Bytes: []byte("payload-b"), SavedAt: time.Now()},
	}
	err := p.Save(context.Background(), snaps)
	require.NoError(t, err)
	got, err := p.Load(context.Background())
	require.NoError(t, err)
	assert.Len(t, got, 2)
}

func TestMemoryPersistence_LoadEmpty(t *testing.T) {
	p := NewMemoryPersistence()
	got, err := p.Load(context.Background())
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestFilePersistence_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/cache.json"
	p := NewFilePersistence(path)

	now := time.Now().Truncate(time.Second).UTC()
	snaps := []NamedSnapshot{
		{Name: "media.sounds", Bytes: []byte(`{"a":1}`), SavedAt: now},
	}
	require.NoError(t, p.Save(context.Background(), snaps))

	got, err := p.Load(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "media.sounds", got[0].Name)
	assert.Equal(t, []byte(`{"a":1}`), got[0].Bytes)
	assert.True(t, got[0].SavedAt.Equal(now))
}

func TestFilePersistence_LoadMissingFile(t *testing.T) {
	p := NewFilePersistence(t.TempDir() + "/missing.json")
	got, err := p.Load(context.Background())
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestFilePersistence_LoadCorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/cache.json"
	require.NoError(t, os.WriteFile(path, []byte("not json"), 0o600))
	p := NewFilePersistence(path)
	_, err := p.Load(context.Background())
	assert.Error(t, err)
}
