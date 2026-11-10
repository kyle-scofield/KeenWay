package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"keenway/internal/store"
)

// go test ./internal/cli -run Preview -v  (KEENWAY_PREVIEW=192.168.1.1 to show live status)
func TestBannerPreview(t *testing.T) {
	host := os.Getenv("KEENWAY_PREVIEW")
	if host == "" {
		t.Skip("set KEENWAY_PREVIEW to the router IP")
	}
	st, _ := store.OpenAt(filepath.Join(t.TempDir(), "s.json"))
	a := New(st, "v0.1.1")
	a.color = true
	a.E.MihomoHost = host
	a.banner(context.Background())
}
