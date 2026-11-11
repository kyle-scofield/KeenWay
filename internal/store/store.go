// Package store keeps the app's settings in the user's config folder.
package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

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
	Panel       Panel    `json:"panel"`
}

// Panel is the optional web panel that runs on the router itself (keenway panel on).
type Panel struct {
	Port     int    `json:"port,omitempty"`
	PassHash string `json:"passHash,omitempty"` // bcrypt
	// Session is the sign-in cookie value; it survives restarts and updates and is
	// replaced when the password changes (which signs everyone out).
	Session string `json:"session,omitempty"`
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
	// mod is the file's modification time as last read or written: when another process
	// (the router's web panel and the terminal program share the file) changes it, it is re-read.
	mod time.Time
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
	if fi, err := os.Stat(path); err == nil {
		s.mod = fi.ModTime()
	}
	return s, nil
}

// refresh re-reads the file if another process has changed it. Call with s.mu held.
func (s *Store) refresh() {
	fi, err := os.Stat(s.path)
	if err != nil || fi.ModTime().Equal(s.mod) {
		return
	}
	b, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var fresh State
	if json.Unmarshal(b, &fresh) == nil {
		s.state = fresh
		gen.TidySiteNames(&s.state.Settings)
	}
	s.mod = fi.ModTime()
}

// Get returns a copy of the state.
func (s *Store) Get() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refresh()
	return s.state
}

// Update changes the state and saves it atomically; the file is readable only by the user.
func (s *Store) Update(fn func(*State)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refresh()
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
	if err := os.Rename(tmp, s.path); err != nil {
		return err
	}
	if fi, err := os.Stat(s.path); err == nil {
		s.mod = fi.ModTime()
	}
	return nil
}
