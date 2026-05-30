package cmd

import (
	"os"
	"path/filepath"
	"testing"

	amazondevices "github.com/Fishwaldo/goamazondevices"
)

func TestResolvePaths_OSStandardDefaults(t *testing.T) {
	cfgDir := t.TempDir()
	cacheDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	t.Setenv("XDG_CACHE_HOME", cacheDir)

	p, err := resolvePaths("", "", "")
	if err != nil {
		t.Fatalf("resolvePaths: %v", err)
	}
	if want := filepath.Join(cfgDir, appDirName, "config.yaml"); p.config != want {
		t.Errorf("config = %q, want %q", p.config, want)
	}
	if want := filepath.Join(cfgDir, appDirName, "session.json"); p.session != want {
		t.Errorf("session = %q, want %q", p.session, want)
	}
	if want := filepath.Join(cacheDir, appDirName, "cache.json"); p.cache != want {
		t.Errorf("cache = %q, want %q", p.cache, want)
	}
}

func TestResolvePaths_FlagOverrides(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	p, err := resolvePaths("/c/my.yaml", "/s/my.json", "/k/my.cache")
	if err != nil {
		t.Fatalf("resolvePaths: %v", err)
	}
	if p.config != "/c/my.yaml" || p.session != "/s/my.json" || p.cache != "/k/my.cache" {
		t.Errorf("got %+v, want flag values", p)
	}
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := loadStoreAt(storePaths{
		config:  filepath.Join(dir, "config.yaml"),
		session: filepath.Join(dir, "session.json"),
		cache:   filepath.Join(dir, "cache.json"),
	})
	if err != nil {
		t.Fatalf("loadStoreAt: %v", err)
	}
	return s
}

func TestConfigSetGetRoundTrip(t *testing.T) {
	s := newTestStore(t)
	if err := s.Set("default-device", "G123"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got, ok := s.Get("default-device"); !ok || got != "G123" {
		t.Fatalf("Get = %q,%v; want G123,true", got, ok)
	}
	s2, err := loadStoreAt(s.paths)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got, ok := s2.Get("default-device"); !ok || got != "G123" {
		t.Fatalf("reloaded Get = %q,%v; want G123,true", got, ok)
	}
}

func TestConfigUnset(t *testing.T) {
	s := newTestStore(t)
	_ = s.Set("default-device", "G123")
	if err := s.Unset("default-device"); err != nil {
		t.Fatalf("Unset: %v", err)
	}
	if _, ok := s.Get("default-device"); ok {
		t.Fatal("expected default-device cleared")
	}
}

func TestConfigSetEmptyClears(t *testing.T) {
	s := newTestStore(t)
	_ = s.Set("default-device", "G123")
	if err := s.Set("default-device", ""); err != nil {
		t.Fatalf("Set empty: %v", err)
	}
	if _, ok := s.Get("default-device"); ok {
		t.Fatal("expected empty Set to clear the key")
	}
}

func TestConfigSetUnknownKeyRejected(t *testing.T) {
	s := newTestStore(t)
	if err := s.Set("bogus", "x"); err == nil {
		t.Fatal("expected error for unknown key")
	}
}

func TestSessionRoundTrip(t *testing.T) {
	s := newTestStore(t)
	if err := s.SaveSession(&amazondevices.Session{}); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	info, err := os.Stat(s.paths.session)
	if err != nil {
		t.Fatalf("stat session: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("session perm = %o, want 600", perm)
	}
	if out := s.LoadSession(); out == nil {
		t.Fatal("LoadSession returned nil after save")
	}
}

func TestLoadSessionMissingReturnsNil(t *testing.T) {
	s := newTestStore(t)
	if got := s.LoadSession(); got != nil {
		t.Errorf("LoadSession on missing file = %v, want nil", got)
	}
}

func TestLoad_ConfigKeyOverridesSessionPath(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	customSession := filepath.Join(dir, "custom-session.json")
	if err := os.WriteFile(cfgPath, []byte("session-file: "+customSession+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(cfgPath, "", "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.paths.session != customSession {
		t.Errorf("session = %q, want %q (from config key)", s.paths.session, customSession)
	}
}

func TestLoad_FlagBeatsConfigKey(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	_ = os.WriteFile(cfgPath, []byte("session-file: /from/config.json\n"), 0o600)
	s, err := Load(cfgPath, "/from/flag.json", "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.paths.session != "/from/flag.json" {
		t.Errorf("session = %q, want flag value", s.paths.session)
	}
}
