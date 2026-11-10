// Package core holds what both front-ends (terminal on the router, window on a PC) share:
// building the config from saved settings, importing the router's config, applying, status.
package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"keenway/internal/gen"
	"keenway/internal/links"
	"keenway/internal/mihomo"
	"keenway/internal/router"
	"keenway/internal/store"
)

// Engine ties saved settings to a router connection.
type Engine struct {
	St *store.Store
	// Connect opens a connection to the router (local shell on the router, SSH from a PC).
	Connect func(ctx context.Context) (*router.Conn, error)
	// MihomoHost is where the mihomo API answers: 127.0.0.1 on the router, the router's IP from a PC.
	MihomoHost string
}

// Hash identifies a config so the app can tell whether it has been applied.
func Hash(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// Proxies turns saved keys and imported servers into mihomo proxies.
func Proxies(st store.State) ([]*links.Proxy, error) {
	var out []*links.Proxy
	for _, raw := range st.RawProxies {
		p, err := gen.ProxyFromYAML(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	for _, k := range st.Keys {
		for _, it := range links.Parse(k) {
			if it.Err != nil {
				return nil, fmt.Errorf("ключ не распознан: %v", it.Err)
			}
			if it.Proxy != nil {
				out = append(out, it.Proxy)
			}
		}
	}
	return out, nil
}

// Build turns saved state into config.yaml.
func Build(st store.State) ([]byte, error) {
	settings := st.Settings
	proxies, err := Proxies(st)
	if err != nil {
		return nil, err
	}
	settings.Proxies = proxies
	return gen.Build(settings)
}

// Pending reports whether saved settings differ from what was last applied.
func Pending(st store.State) bool {
	cfg, err := Build(st)
	return err == nil && Hash(cfg) != st.LastApplied
}

// Import reads the router's live config into the saved settings (first run).
func (e *Engine) Import(ctx context.Context, c *router.Conn) ([]string, error) {
	live, err := c.CurrentConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("не удалось прочитать конфиг роутера: %v", err)
	}
	settings, warnings, err := gen.Import(live)
	if err != nil {
		return nil, err
	}
	var raw []string
	for _, p := range settings.Proxies {
		raw = append(raw, gen.ProxyToYAML(p))
	}
	settings.Proxies = nil
	mac, _ := c.MAC(ctx)
	err = e.St.Update(func(st *store.State) {
		st.Settings, st.RawProxies, st.Imported = settings, raw, true
		if mac != "" {
			st.RouterMAC = mac
		}
		if cfg, err := Build(*st); err == nil {
			st.LastApplied = Hash(cfg)
		}
	})
	return warnings, err
}

// Mihomo returns an API client, trying the saved panel password and then none
// (a router never configured by KeenWay has no password).
func (e *Engine) Mihomo(ctx context.Context) *mihomo.Client {
	secret := e.St.Get().Settings.Secret
	for _, s := range []string{secret, ""} {
		if _, err := mihomo.New(e.MihomoHost, s).Version(ctx); err == nil {
			return mihomo.New(e.MihomoHost, s)
		}
	}
	return mihomo.New(e.MihomoHost, secret)
}

// Apply builds, validates and applies the config with backup and automatic rollback.
func (e *Engine) Apply(ctx context.Context, dry bool, progress func(router.Step)) error {
	st := e.St.Get()
	cfg, err := Build(st)
	if err != nil {
		return err
	}
	c, err := e.Connect(ctx)
	if err != nil {
		return err
	}
	defer c.Close()
	running := e.Mihomo(ctx).Secret
	if err := c.Apply(ctx, cfg, router.ApplyOptions{Secret: running, NewSecret: st.Settings.Secret, DryRun: dry, Progress: progress}); err != nil {
		return err
	}
	if !dry {
		return e.St.Update(func(st *store.State) { st.LastApplied = Hash(cfg) })
	}
	return nil
}

// DefaultHeaders makes subscription requests look like the router's own mihomo, so
// providers that bind subscriptions to a device ID don't count an extra device.
func DefaultHeaders(st store.State) map[string]string {
	h := map[string]string{"User-Agent": "mihomo/v1.19.30", "x-device-os": "Keenetic OS"}
	if st.RouterMAC != "" {
		h["x-hwid"] = st.RouterMAC
	}
	return h
}

// ParseTarget turns "youtube.com", "https://www.youtube.com/watch?v=1" or "1.2.3.4:8443"
// into a host and port for a route check.
func ParseTarget(q string) (string, int, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return "", 0, fmt.Errorf("укажите сайт, например youtube.com")
	}
	if !strings.Contains(q, "://") {
		q = "https://" + q
	}
	u, err := url.Parse(q)
	if err != nil || u.Hostname() == "" {
		return "", 0, fmt.Errorf("не похоже на адрес сайта: %s", q)
	}
	port := 443
	if u.Scheme == "http" {
		port = 80
	}
	if p := u.Port(); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n > 0 && n < 65536 {
			port = n
		}
	}
	return strings.ToLower(u.Hostname()), port, nil
}

// Profiles switch every service tile at once through the mihomo API: instant, no apply.
var Profiles = []struct{ ID, Name, About string }{
	{"normal", "Обычный", "как настроено: каждый сервис по умолчанию"},
	{"vpn", "Всё через VPN", "все сервисы и остальные сайты через VPN"},
	{"direct", "Без VPN", "всё напрямую, VPN не используется"},
	{"games", "Игровой", "игры, Amazon и Cloudflare напрямую ради пинга, остальное как обычно"},
}

var gameGroups = map[string]bool{"PlayStation": true, "Xbox": true, "Steam": true, "Игры и лаунчеры": true, "Amazon": true, "Cloudflare": true}

// ApplyProfile selects options in all service groups; it returns how many groups changed.
func ApplyProfile(ctx context.Context, m *mihomo.Client, profile string) (int, error) {
	groups, err := m.Groups(ctx)
	if err != nil {
		return 0, err
	}
	isGroup := map[string]bool{}
	for _, g := range groups {
		isGroup[g.Name] = true
	}
	changed := 0
	for _, g := range groups {
		if g.Type != "Selector" || g.Hidden || g.Name == gen.GroupVPN || g.Name == gen.GroupManual || g.Name == "GLOBAL" || len(g.Options) == 0 {
			continue
		}
		// service tiles offer VPN / DIRECT; skip selectors of servers
		if !slices.Contains(g.Options, gen.GroupVPN) || !slices.Contains(g.Options, "DIRECT") {
			continue
		}
		want := g.Options[0]
		switch profile {
		case "normal":
		case "vpn":
			want = gen.GroupVPN
		case "direct":
			want = "DIRECT"
		case "games":
			if gameGroups[g.Name] {
				want = "DIRECT"
			}
		default:
			return 0, fmt.Errorf("неизвестный профиль %q", profile)
		}
		if g.Now == want {
			continue
		}
		if err := m.Select(ctx, g.Name, want); err != nil {
			return changed, fmt.Errorf("%s: %v", g.Name, err)
		}
		changed++
	}
	return changed, nil
}
