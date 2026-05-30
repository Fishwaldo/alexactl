// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Fishwaldo/alexactl"
	"github.com/spf13/viper"
)

// appDirName is the per-app subdirectory under the OS config and cache dirs.
const appDirName = "alexactl"

type storePaths struct {
	config  string
	session string
	cache   string
}

// xdgOrOS returns the value of the XDG env var if set, otherwise falls back
// to the OS-standard base directory via the provided fallback function. This
// makes resolvePaths testable on macOS where os.UserConfigDir /
// os.UserCacheDir do not honor XDG_CONFIG_HOME / XDG_CACHE_HOME.
func xdgOrOS(xdgEnv string, fallback func() (string, error)) (string, error) {
	if v := os.Getenv(xdgEnv); v != "" {
		return v, nil
	}
	return fallback()
}

// resolvePaths computes the config, session, and cache paths. A non-empty
// flag overrides everything for that file; otherwise the OS-standard location
// under appDirName is used. Config-key overrides (session-file / cache-file)
// are applied later by Load, after the config file has been read.
func resolvePaths(configFlag, sessionFlag, cacheFlag string) (storePaths, error) {
	var p storePaths
	cfgBase, err := xdgOrOS("XDG_CONFIG_HOME", os.UserConfigDir)
	if err != nil {
		return p, fmt.Errorf("resolve config dir: %w", err)
	}
	cacheBase, err := xdgOrOS("XDG_CACHE_HOME", os.UserCacheDir)
	if err != nil {
		return p, fmt.Errorf("resolve cache dir: %w", err)
	}
	if configFlag != "" {
		p.config = configFlag
	} else {
		p.config = filepath.Join(cfgBase, appDirName, "config.yaml")
	}
	if sessionFlag != "" {
		p.session = sessionFlag
	} else {
		p.session = filepath.Join(cfgBase, appDirName, "session.json")
	}
	if cacheFlag != "" {
		p.cache = cacheFlag
	} else {
		p.cache = filepath.Join(cacheBase, appDirName, "cache.json")
	}
	return p, nil
}

// configKeys is the allowlist of settable config keys.
var configKeys = map[string]bool{
	"default-device": true,
	"session-file":   true,
	"cache-file":     true,
	"site":           true,
}

// Store is the single persistence path for the CLI: config settings, the
// library Session, and the on-disk cache file.
type Store struct {
	paths storePaths
	v     *viper.Viper
}

// loadStoreAt builds a Store from resolved paths, reading the config file if
// present. A missing config file is not an error.
func loadStoreAt(p storePaths) (*Store, error) {
	v := viper.New()
	v.SetConfigType("yaml")
	v.SetConfigFile(p.config)
	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !os.IsNotExist(err) && !errors.As(err, &notFound) {
			return nil, fmt.Errorf("read config %s: %w", p.config, err)
		}
	}
	return &Store{paths: p, v: v}, nil
}

// Get returns a config value and whether it is set (non-empty).
func (s *Store) Get(key string) (string, bool) {
	val := s.v.GetString(key)
	return val, val != ""
}

// Set validates key against the allowlist and persists the value. An empty
// value clears the key.
func (s *Store) Set(key, val string) error {
	if !configKeys[key] {
		return fmt.Errorf("unknown config key %q (valid: default-device, session-file, cache-file, site)", key)
	}
	if val == "" {
		return s.Unset(key)
	}
	s.v.Set(key, val)
	return s.write()
}

// Unset clears a config key and persists. viper has no delete, so rebuild
// without the key.
func (s *Store) Unset(key string) error {
	if !configKeys[key] {
		return fmt.Errorf("unknown config key %q", key)
	}
	all := s.v.AllSettings()
	delete(all, key)
	nv := viper.New()
	nv.SetConfigType("yaml")
	nv.SetConfigFile(s.paths.config)
	for k, val := range all {
		nv.Set(k, val)
	}
	s.v = nv
	return s.write()
}

// write persists the config file, creating its directory.
func (s *Store) write() error {
	if err := os.MkdirAll(filepath.Dir(s.paths.config), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	if err := s.v.WriteConfigAs(s.paths.config); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

// ConfigPath returns the resolved config file path.
func (s *Store) ConfigPath() string { return s.paths.config }

// LoadSession reads the persisted Session, or returns nil if absent or
// unreadable (caller then logs in fresh).
func (s *Store) LoadSession() *alexactl.Session {
	b, err := os.ReadFile(s.paths.session)
	if err != nil {
		return nil
	}
	var sess alexactl.Session
	if err := jsonUnmarshal(b, &sess); err != nil {
		return nil
	}
	return &sess
}

// SaveSession writes the Session as 0600 JSON, creating its directory.
func (s *Store) SaveSession(sess *alexactl.Session) error {
	if err := os.MkdirAll(filepath.Dir(s.paths.session), 0o700); err != nil {
		return fmt.Errorf("create session dir: %w", err)
	}
	b, err := jsonMarshalIndent(sess)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}
	if err := os.WriteFile(s.paths.session, b, 0o600); err != nil {
		return fmt.Errorf("write session: %w", err)
	}
	return nil
}

// SessionPath returns the resolved session file path.
func (s *Store) SessionPath() string { return s.paths.session }

// CachePersistence returns a file-backed Persistence at the resolved cache
// path, using the library's atomic file adapter.
func (s *Store) CachePersistence() alexactl.Persistence {
	return alexactl.FilePersistence(s.paths.cache)
}

// Load resolves all paths and reads config, applying override precedence:
// flag > config-key (session-file / cache-file) > OS-standard default. If
// configFlag is empty and ./config.yaml exists in the working directory, it is
// used (backward compatibility).
func Load(configFlag, sessionFlag, cacheFlag string) (*Store, error) {
	if configFlag == "" {
		if _, err := os.Stat("config.yaml"); err == nil {
			configFlag = "config.yaml"
		}
	}
	p, err := resolvePaths(configFlag, sessionFlag, cacheFlag)
	if err != nil {
		return nil, err
	}
	s, err := loadStoreAt(p)
	if err != nil {
		return nil, err
	}
	if sessionFlag == "" {
		if v, ok := s.Get("session-file"); ok {
			s.paths.session = v
		}
	}
	if cacheFlag == "" {
		if v, ok := s.Get("cache-file"); ok {
			s.paths.cache = v
		}
	}
	return s, nil
}
