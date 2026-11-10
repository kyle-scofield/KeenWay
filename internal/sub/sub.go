// Package sub downloads subscriptions and reads what providers report about them
// (traffic used, quota, expiry, title) the way Happ and mihomo do.
package sub

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"keenway/internal/links"
)

// Info is what the provider reports in the "subscription-userinfo" header.
type Info struct {
	Upload   int64     `json:"upload"`
	Download int64     `json:"download"`
	Total    int64     `json:"total"` // 0 = unlimited
	Expire   time.Time `json:"expire"`
	Title    string    `json:"title,omitempty"`
}

// Used is upload + download in bytes.
func (i Info) Used() int64 { return i.Upload + i.Download }

// DaysLeft is whole days until expiry, or -1 when the provider sets no expiry.
func (i Info) DaysLeft(now time.Time) int {
	if i.Expire.IsZero() {
		return -1
	}
	d := i.Expire.Sub(now)
	if d < 0 {
		return 0
	}
	return int(d.Hours() / 24)
}

// ParseUserInfo reads "upload=1; download=2; total=3; expire=1700000000".
func ParseUserInfo(h string) Info {
	var info Info
	for _, part := range strings.Split(h, ";") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		n, _ := strconv.ParseFloat(strings.TrimSpace(v), 64)
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "upload":
			info.Upload = int64(n)
		case "download":
			info.Download = int64(n)
		case "total":
			info.Total = int64(n)
		case "expire":
			if n > 0 {
				info.Expire = time.Unix(int64(n), 0)
			}
		}
	}
	return info
}

// decodeTitle handles "base64:…" profile titles that many panels send.
func decodeTitle(t string) string {
	if rest, ok := strings.CutPrefix(t, "base64:"); ok {
		if b, err := links.DecodeBase64(rest); err == nil {
			return string(b)
		}
	}
	return t
}

// Result is a downloaded and parsed subscription.
type Result struct {
	Info    Info
	HasInfo bool
	Nodes   []string // proxy names, for display and health checks
	Format  string   // "clash", "base64", "plain"
	Errors  []string
}

// Fetch downloads a subscription. headers should match what the router sends
// (User-Agent, x-hwid…) so the provider doesn't count this PC as a new device.
func Fetch(ctx context.Context, url string, headers map[string]string) (*Result, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("неверная ссылка подписки: %v", err)
	}
	req.Header.Set("User-Agent", "mihomo/1.19")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("не удалось скачать подписку: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("сервер подписки ответил %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		return nil, fmt.Errorf("обрыв при скачивании подписки: %v", err)
	}
	r := ParseBody(body)
	if h := resp.Header.Get("subscription-userinfo"); h != "" {
		r.Info = ParseUserInfo(h)
		r.HasInfo = true
	}
	r.Info.Title = decodeTitle(resp.Header.Get("profile-title"))
	return r, nil
}

// ParseBody understands Clash/mihomo YAML, base64 link lists and plain link lists.
func ParseBody(body []byte) *Result {
	r := &Result{}
	var clash struct {
		Proxies []map[string]any `yaml:"proxies"`
	}
	if yaml.Unmarshal(body, &clash) == nil && len(clash.Proxies) > 0 {
		r.Format = "clash"
		for _, p := range clash.Proxies {
			if n, ok := p["name"].(string); ok {
				r.Nodes = append(r.Nodes, n)
			}
		}
		return r
	}
	text := strings.TrimSpace(string(body))
	r.Format = "plain"
	if !strings.Contains(text, "://") {
		if dec, err := base64.StdEncoding.DecodeString(text); err == nil {
			text, r.Format = string(dec), "base64"
		} else if dec, err := links.DecodeBase64(strings.ReplaceAll(text, "\n", "")); err == nil {
			text, r.Format = string(dec), "base64"
		}
	}
	for _, it := range links.Parse(text) {
		switch {
		case it.Err != nil:
			r.Errors = append(r.Errors, it.Err.Error())
		case it.Proxy != nil:
			r.Nodes = append(r.Nodes, it.Proxy.Name())
		}
	}
	return r
}
