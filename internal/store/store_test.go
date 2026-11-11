package store

import (
	"path/filepath"
	"testing"
)

// The router's web panel and the terminal program are two processes sharing one file.
func TestTwoProcessesSeeEachOther(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	a, _ := OpenAt(path)
	a.Update(func(s *State) { s.Settings.BlockAds = false })
	b, _ := OpenAt(path)
	b.Update(func(s *State) { s.Panel.Port = 8091 })
	if a.Get().Panel.Port != 8091 {
		t.Fatal("a did not see b's change")
	}
	a.Update(func(s *State) { s.Settings.Mode = "extended" })
	if got := b.Get(); got.Settings.Mode != "extended" || got.Panel.Port != 8091 {
		t.Fatalf("b: %+v", got.Settings.Mode)
	}
}
