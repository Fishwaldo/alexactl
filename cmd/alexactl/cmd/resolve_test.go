// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package cmd

import (
	"path/filepath"
	"strings"
	"testing"
)

func storeWithDefault(t *testing.T, serial string) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := loadStoreAt(storePaths{
		config:  filepath.Join(dir, "config.yaml"),
		session: filepath.Join(dir, "session.json"),
		cache:   filepath.Join(dir, "cache.json"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if serial != "" {
		if err := s.Set("default-device", serial); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func TestResolveTarget_FlagNameWins(t *testing.T) {
	s := storeWithDefault(t, "G_DEFAULT")
	got, err := resolveTarget("Kitchen", s, devs())
	if err != nil || got != "G1" {
		t.Fatalf("got %q,%v; want G1,nil", got, err)
	}
}

func TestResolveTarget_FlagSerialWins(t *testing.T) {
	s := storeWithDefault(t, "G_DEFAULT")
	got, err := resolveTarget("G2", s, devs())
	if err != nil || got != "G2" {
		t.Fatalf("got %q,%v; want G2,nil", got, err)
	}
}

func TestResolveTarget_FallsBackToDefault(t *testing.T) {
	s := storeWithDefault(t, "G_DEFAULT")
	got, err := resolveTarget("", s, devs())
	if err != nil || got != "G_DEFAULT" {
		t.Fatalf("got %q,%v; want G_DEFAULT,nil", got, err)
	}
}

func TestResolveTarget_NeitherIsError(t *testing.T) {
	s := storeWithDefault(t, "")
	_, err := resolveTarget("", s, devs())
	if err == nil || !strings.Contains(err.Error(), "config set default-device") {
		t.Fatalf("want guidance error, got %v", err)
	}
}
