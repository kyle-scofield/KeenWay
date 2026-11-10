// Package store keeps the app's settings in the user's config folder.
package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"keenway/internal/gen"
	"keenway/internal/router"
)

// State is everything the app remembers between runs.
type State struct {
	Router         router.Target `json:"router"`
	RouterPassword string        `json:"routerPassword,omitempty"` // only when the user ticked "remember"
	Settings       gen.Settings  `json:"settings"`
	// Keys holds individual share links / WireGuard configs as the user pasted them;
	// they are parsed into Settings.Proxies when building the config.
	Keys []string `json:"keys"`
	// RawProxies are mihomo proxy definitions (YAML) imported from the router's config.
	RawProxies  []string `json:"rawProxies,omitempty"`
	RouterMAC   string   `json:"routerMac,omitempty"`
	Imported    bool     `json:"imported"`    // the router's existing config has been read once
	LastApplied string   `json:"lastApplied"` // sha256 of the last config applied successfully
	Notify      Notify   `json:"notify"`
}

// Notify holds the Telegram notification settings (installed on the router).
type Notify struct {
	Enabled bool   `json:"enabled"`
	Token   string `json:"token,omitempty"`
	ChatID  string `json:"chatId,omitempty"`
	Days    int    `json:"days,omitempty"`
}

// Store guards the state file.
type Store struct {
	mu    sync.Mutex
	path  string
	state State
}

// Open loads the state (or starts empty) from <user config dir>/KeenWay/state.json.
func Open() (*Store, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	return OpenAt(filepath.Join(dir, "KeenWay", "state.json"))
}

func OpenAt(path string) (*Store, error) {
	s := &Store{path: path}
	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if len(b) > 0 {
		if err := json.Unmarshal(b, &s.state); err != nil {
			// Keep a broken file for inspection instead of losing it.
			os.Rename(path, path+".broken")
		}
	}
	if s.state.Router.Host == "" {
		s.state.Router.Host = "192.168.1.1"
		s.state.Router.User = "admin"
	}
	if s.state.Settings.Mode == "" {
		s.state.Settings.Mode = "simple"
		s.state.Settings.Services = gen.DefaultServiceIDs()
		s.state.Settings.BlockAds = true
	}
	gen.TidySiteNames(&s.state.Settings)
	return s, nil
}

// Get returns a copy of the state.
func (s *Store) Get() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// Update changes the state and saves it atomically; the file is readable only by the user.
func (s *Store) Update(fn func(*State)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.state)
	b, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
