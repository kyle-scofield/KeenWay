package gen

import (
	"os"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"keenway/internal/links"
)

// userSettings reproduces the config the user runs today (rockblack "Простой" + subscription).
func userSettings() Settings {
	return Settings{
		Mode: "simple",
		Subscriptions: []Subscription{{
			Name: "WEBNETVPN", URL: "https://SUBSCRIPTION_URL", Interval: 3600,
			Headers: map[string]string{"User-Agent": "mihomo/v1.19.30", "x-hwid": "AABBCCDDEEFF", "x-device-os": "Keenetic OS", "x-ver-os": "", "x-device-model": ""},
		}},
		Services: DefaultServiceIDs(),
		BlockAds: true,
		Sites: []CustomSite{
			{Name: "Мои заблокированные", Target: "blocked", Domains: []string{"cdn-service.space", "kino.watch", "teleos.club", "pushbr.com", "truenas.com", "truenasconnect.net", "smarttvcdn.online", "srvkp.com", "wwsga.me", "gran-turismo.com"}, Ports: []string{"30000-31000"}},
			{Name: "Мои недоступные", Target: "geoblock", Domains: []string{"4pda.to", "happ-proxy.com"}},
		},
	}
}

type cfgDoc struct {
	ProxyGroups   []map[string]any          `yaml:"proxy-groups"`
	RuleProviders map[string]map[string]any `yaml:"rule-providers"`
	Rules         []string                  `yaml:"rules"`
	Providers     map[string]map[string]any `yaml:"proxy-providers"`
}

func load(t *testing.T, b []byte) cfgDoc {
	t.Helper()
	var d cfgDoc
	if err := yaml.Unmarshal(b, &d); err != nil {
		t.Fatalf("yaml: %v", err)
	}
	return d
}

// The generated config must match what the user already runs, apart from known cleanups.
func TestMatchesUsersCurrentConfig(t *testing.T) {
	ref, err := os.ReadFile("../../reference/rockblack-simple-subscription.yaml")
	if err != nil {
		t.Skip("reference config not available")
	}
	out, err := Build(userSettings())
	if err != nil {
		t.Fatal(err)
	}
	want, got := load(t, ref), load(t, out)

	if len(want.ProxyGroups) != len(got.ProxyGroups) {
		t.Fatalf("groups: want %d, got %d", len(want.ProxyGroups), len(got.ProxyGroups))
	}
	for i := range want.ProxyGroups {
		w, g := want.ProxyGroups[i], got.ProxyGroups[i]
		for _, k := range []string{"name", "type", "icon", "include-all", "hidden", "url", "interval"} {
			if w[k] != g[k] {
				t.Errorf("group %v: %s = %v, want %v", w["name"], k, g[k], w[k])
			}
		}
		// tolerance is deliberately lower than the reference's 150 so Auto-Best follows the fastest server
		if w["tolerance"] != nil && g["tolerance"] != autoTolerance {
			t.Errorf("group %v: tolerance = %v, want %v", w["name"], g["tolerance"], autoTolerance)
		}
		if strings.Join(toStrings(w["proxies"]), "|") != strings.Join(toStrings(g["proxies"]), "|") {
			t.Errorf("group %v proxies = %v, want %v", w["name"], g["proxies"], w["proxies"])
		}
	}

	// Unused providers in the reference (youtubeip, ubisoft) are intentionally dropped.
	for id, w := range want.RuleProviders {
		g, ok := got.RuleProviders[id]
		if !ok {
			if id != "youtubeip" && id != "ubisoft" {
				t.Errorf("rule-provider %s missing", id)
			}
			continue
		}
		for _, k := range []string{"type", "behavior", "format", "url", "path"} {
			if w[k] != g[k] {
				t.Errorf("rule-provider %s: %s = %v, want %v", id, k, g[k], w[k])
			}
		}
	}

	// The catalog may add lists to a service, but must keep every rule of the reference.
	if missing := diff(diff(sorted(want.Rules), sorted(got.Rules)), droppedTemplateRules); len(missing) > 0 {
		t.Errorf("rules missing: %v", missing)
	}
	for _, r := range got.Rules {
		if strings.Contains(r, "github.com,DIRECT") {
			t.Errorf("GitHub must follow its tile, found %s", r)
		}
	}
	if got.Rules[len(got.Rules)-1] != "MATCH,Остальное" || got.Rules[0] != "RULE-SET,geosite-private,DIRECT" {
		t.Errorf("rule order: first=%s last=%s", got.Rules[0], got.Rules[len(got.Rules)-1])
	}
	h := got.Providers["WEBNETVPN"]["header"].(map[string]any)
	if toStrings(h["x-hwid"])[0] != "AABBCCDDEEFF" {
		t.Errorf("hwid header lost: %v", h)
	}
}

func TestDevicesSitesAndExtended(t *testing.T) {
	vless := links.ParseLine("vless://id@1.2.3.4:443?security=tls&sni=a#My VPS").Proxy
	s := Settings{
		Mode:     "extended",
		Proxies:  []*links.Proxy{vless, vless},
		Services: []string{"youtube"},
		Sites:    []CustomSite{{Name: "Кинопоиск", Icon: "https://example.com/kp.png", Policy: PolicyGroup, Domains: []string{"https://www.kinopoisk.ru/film/1"}}},
		Devices:  []DeviceRule{{Name: "ПК", IP: "192.168.1.84", Policy: PolicyDirect}, {Name: "TV", IP: "192.168.1.50", Policy: PolicyVPN}},
		Secret:   "s3cret",
	}
	out, err := Build(s)
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	for _, want := range []string{
		"SRC-IP-CIDR,192.168.1.84/32,DIRECT", "SRC-IP-CIDR,192.168.1.50/32,VPN",
		"DOMAIN-SUFFIX,kinopoisk.ru,Кинопоиск", "name: Кинопоиск", "icon: https://example.com/kp.png",
		"type: fallback", "secret: s3cret",
		"name: My VPS (2)",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in generated config", want)
		}
	}
	d := load(t, out)
	names := map[string]bool{}
	for _, g := range d.ProxyGroups {
		names[g["name"].(string)] = true
	}
	for _, n := range []string{GroupVPN, GroupAutoBest, GroupFallback, GroupManual, "YouTube", "Кинопоиск", GroupOther} {
		if !names[n] {
			t.Errorf("group %q missing", n)
		}
	}
	if d.Rules[3] != "SRC-IP-CIDR,192.168.1.84/32,DIRECT" {
		t.Errorf("device rules must come right after local rules, got %v", d.Rules[:5])
	}
}

func TestValidation(t *testing.T) {
	bad := []Settings{
		{},
		{Subscriptions: []Subscription{{Name: "a,b", URL: "https://x"}}},
		{Subscriptions: []Subscription{{Name: "a", URL: "ftp://x"}}},
		{Subscriptions: []Subscription{{Name: "a", URL: "https://x"}}, Sites: []CustomSite{{Name: "x", Domains: []string{"not a domain"}}}},
		{Subscriptions: []Subscription{{Name: "a", URL: "https://x"}}, Devices: []DeviceRule{{Name: "x", IP: "999.1.1.1"}}},
	}
	for i, s := range bad {
		if _, err := Build(s); err == nil {
			t.Errorf("case %d: expected validation error", i)
		}
	}
}

func toStrings(v any) []string {
	var out []string
	if l, ok := v.([]any); ok {
		for _, x := range l {
			out = append(out, x.(string))
		}
	}
	return out
}

func sorted(s []string) []string {
	c := append([]string(nil), s...)
	sort.Strings(c)
	return c
}

func diff(a, b []string) []string {
	in := map[string]int{}
	for _, x := range b {
		in[x]++
	}
	var out []string
	for _, x := range a {
		if in[x] > 0 {
			in[x]--
		} else {
			out = append(out, x)
		}
	}
	return out
}

func TestSubscriptionNamedLikeGroup(t *testing.T) {
	out, err := Build(Settings{Mode: "simple", Subscriptions: []Subscription{{Name: "VPN", URL: "https://example.com/s"}}, Services: DefaultServiceIDs()})
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Providers map[string]any   `yaml:"proxy-providers"`
		Groups    []map[string]any `yaml:"proxy-groups"`
	}
	if err := yaml.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	if _, ok := doc.Providers["VPN"]; ok {
		t.Fatal("provider must not be named VPN: it clashes with the VPN group")
	}
	if _, ok := doc.Providers["VPN подписка"]; !ok {
		t.Fatalf("providers: %v", doc.Providers)
	}
}

func TestAutoExclude(t *testing.T) {
	base := Settings{Mode: "extended", Subscriptions: []Subscription{{Name: "s", URL: "https://x"}}}
	for _, c := range []struct {
		list []string
		want string
	}{{nil, "🇷🇺"}, {[]string{}, ""}, {[]string{"🇷🇺", "LTE", "a.b"}, `🇷🇺|LTE|a\.b`}} {
		s := base
		s.AutoExclude = c.list
		out, err := Build(s)
		if err != nil {
			t.Fatal(err)
		}
		d := load(t, out)
		for _, g := range d.ProxyGroups {
			if g["name"] == GroupAutoBest {
				got, _ := g["exclude-filter"].(string)
				if got != c.want {
					t.Errorf("%v: exclude-filter %q, want %q", c.list, got, c.want)
				}
			}
		}
		// importing back gives the same list
		imp, _, err := Import(out)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Join(imp.AutoExclude, "|") != strings.Join(append([]string{}, orDefault(c.list)...), "|") {
			t.Errorf("import: %v, want %v", imp.AutoExclude, c.list)
		}
	}
}

func orDefault(l []string) []string {
	if l == nil {
		return DefaultAutoExclude
	}
	return l
}
