// Package update finds a newer KeenWay release on GitHub and installs it.
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Release is the newest published version.
type Release struct {
	Tag   string `json:"tag"`
	URL   string `json:"url"`   // release page
	Notes string `json:"notes"` // release text
}

var (
	cacheMu   sync.Mutex
	cached    *Release
	cachedErr error
	cachedAt  time.Time
)

// Latest asks GitHub for the newest release (cached for an hour; errors for 10 minutes).
func Latest(ctx context.Context, repo string) (*Release, error) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	if (cached != nil && time.Since(cachedAt) < time.Hour) || (cachedErr != nil && time.Since(cachedAt) < 10*time.Minute) {
		return cached, cachedErr
	}
	cached, cachedErr = fetchLatest(ctx, repo)
	cachedAt = time.Now()
	return cached, cachedErr
}

func fetchLatest(ctx context.Context, repo string) (*Release, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/"+repo+"/releases/latest", nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GitHub недоступен: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("релизы не найдены (репозиторий закрыт или релизов ещё нет)")
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub ответил %s", res.Status)
	}
	var r struct {
		Tag  string `json:"tag_name"`
		URL  string `json:"html_url"`
		Body string `json:"body"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&r); err != nil {
		return nil, err
	}
	return &Release{Tag: r.Tag, URL: r.URL, Notes: r.Body}, nil
}

// Newer reports whether version a is newer than b ("v0.2.1" > "v0.2.0"). A dev build is never older.
func Newer(a, b string) bool {
	pa, okA := parse(a)
	pb, okB := parse(b)
	if !okA || !okB {
		return false
	}
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] > pb[i]
		}
	}
	return false
}

func parse(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) == 0 || len(parts) > 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// AssetName is the release file for this computer: the window program, or the Windows
// terminal build when terminal is set.
func AssetName(terminal bool) (string, error) {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "windows/amd64":
		if terminal {
			return "keenway-windows-terminal.exe", nil
		}
		return "KeenWay-windows.exe", nil
	case "linux/amd64":
		return "keenway-linux-amd64", nil
	case "linux/arm64":
		return "keenway-linux-arm64", nil
	case "darwin/amd64":
		return "keenway-darwin-amd64", nil
	case "darwin/arm64":
		return "keenway-darwin-arm64", nil
	}
	return "", fmt.Errorf("для %s/%s обновление скачайте вручную", runtime.GOOS, runtime.GOARCH)
}

// ReplaceSelf downloads the release file, checks its checksum and puts it in place of the
// running program. The old file is kept as <name>.old (Windows can't overwrite a running exe)
// and removed on the next start by CleanOld.
func ReplaceSelf(ctx context.Context, repo, tag string, terminal bool) (string, error) {
	asset, err := AssetName(terminal)
	if err != nil {
		return "", err
	}
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	if p, err := filepath.EvalSymlinks(self); err == nil {
		self = p
	}
	base := "https://github.com/" + repo + "/releases/download/" + tag + "/" + asset
	data, err := download(ctx, base, 200<<20)
	if err != nil {
		return "", err
	}
	if sum, err := download(ctx, base+".sha256", 1024); err == nil {
		want := strings.Fields(string(sum))
		got := sha256.Sum256(data)
		if len(want) == 0 || !strings.EqualFold(want[0], hex.EncodeToString(got[:])) {
			return "", fmt.Errorf("контрольная сумма не совпала — файл повреждён, обновление отменено")
		}
	}
	tmp := self + ".new"
	if err := os.WriteFile(tmp, data, 0o755); err != nil {
		return "", fmt.Errorf("нет прав записать %s: %v", tmp, err)
	}
	old := self + ".old"
	os.Remove(old)
	if err := os.Rename(self, old); err != nil {
		os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, self); err != nil {
		os.Rename(old, self)
		return "", err
	}
	return self, nil
}

// Restart starts the given program with the same arguments; the caller then exits.
func Restart(path string, args []string) error {
	cmd := exec.Command(path, args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Start()
}

// CleanOld removes the previous version left by ReplaceSelf.
func CleanOld() {
	if self, err := os.Executable(); err == nil {
		os.Remove(self + ".old")
	}
}

func download(ctx context.Context, url string, limit int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("не удалось скачать обновление: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("не удалось скачать %s: %s", url, res.Status)
	}
	return io.ReadAll(io.LimitReader(res.Body, limit))
}
