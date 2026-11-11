// Package gen builds a mihomo config.yaml for XKeen in the rockblack generator's layout,
// plus the user's own sites, icons and per-device rules.
package gen

import (
	"fmt"
	"net/netip"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"keenway/internal/links"
)

const (
	GroupVPN      = "VPN"
	GroupFastest  = "⚡️ Fastest"
	GroupAutoBest = "🚀 Auto-Best"
	GroupFallback = "🛡️ Fallback"
	GroupManual   = "⚙️ Manual"
	GroupOther    = "Остальное"
)

// Subscription is a provider URL that mihomo downloads and refreshes on the router.
type Subscription struct {
	Name     string            `json:"name"`
	URL      string            `json:"url"`
	Interval int               `json:"interval"` // seconds
	Headers  map[string]string `json:"headers,omitempty"`
}

// Policy says where matching traffic goes.
type Policy string

const (
	PolicyVPN    Policy = "vpn"
	PolicyDirect Policy = "direct"
	PolicyBlock  Policy = "block"
	PolicyGroup  Policy = "group" // own tile in zashboard, switchable there
)

// CustomSite is a user-defined service: domains/IPs, optional icon, and where they go.
type CustomSite struct {
	Name    string   `json:"name"`
	Icon    string   `json:"icon,omitempty"`
	Domains []string `json:"domains"`
	IPs     []string `json:"ips,omitempty"`
	Ports   []string `json:"ports,omitempty"` // destination ports or ranges, e.g. "30000-31000"
	Policy  Policy   `json:"policy"`
	// Target sends the site to an existing service group (e.g. "blocked") instead of a policy.
	Target string `json:"target,omitempty"`
}

// DeviceRule routes everything from one LAN device directly or via VPN.
type DeviceRule struct {
	Name   string `json:"name"`
	IP     string `json:"ip"`
	Policy Policy `json:"policy"`
}

// Settings is everything the user chose in the app.
type Settings struct {
	Mode          string          `json:"mode"` // "simple" | "extended"
	Subscriptions []Subscription  `json:"subscriptions"`
	Proxies       []*links.Proxy  `json:"-"`
	Services      []string        `json:"services"`
	ServiceVPN    map[string]bool `json:"serviceVPN,omitempty"` // per-service override of the default choice
	Sites         []CustomSite    `json:"sites"`
	Devices       []DeviceRule    `json:"devices"`
	Secret        string          `json:"secret,omitempty"` // password for the mihomo panel (port 9090)
	BlockAds      bool            `json:"blockAds"`
	// AutoExclude lists words/flags of servers left out of automatic choice
	// (Auto-Best, Fallback, Fastest); they stay selectable by hand. nil = default (🇷🇺).
	AutoExclude []string `json:"autoExclude"`
}

// Ports of the transparent-proxy inbounds XKeen sends traffic to (XKeen's defaults).
const (
	TProxyPort = 1181
	RedirPort  = 1182
)

// autoTolerance is how much faster (ms) another server must be before url-test switches to it.
// It only stops flapping between near-equal servers; with 150 a 76 ms server never replaced a 203 ms one.
const autoTolerance = 30

// DefaultAutoExclude keeps Russian servers out of automatic choice. In subscriptions they are
// "LTE / whitelist bypass" exits inside Russia: the lowest ping, so url-test would always pick
// them, yet nothing blocked opens through them.
var DefaultAutoExclude = []string{"🇷🇺"}

// excludeFilter builds the mihomo exclude-filter regexp ("" = no filter).
func (s *Settings) excludeFilter() string {
	list := s.AutoExclude
	if list == nil {
		list = DefaultAutoExclude
	}
	var parts []string
	for _, w := range list {
		if w = strings.TrimSpace(w); w != "" {
			parts = append(parts, regexp.QuoteMeta(w))
		}
	}
	return strings.Join(parts, "|")
}

var groupName = regexp.MustCompile(`^[^,\n\r]{1,40}$`)

// Validate catches input that would produce a broken config.
func (s *Settings) Validate() error {
	if len(s.Subscriptions) == 0 && len(s.Proxies) == 0 {
		return fmt.Errorf("добавьте хотя бы один ключ или подписку")
	}
	seen := map[string]bool{}
	for _, sub := range s.Subscriptions {
		if !groupName.MatchString(sub.Name) || strings.ContainsAny(sub.Name, "/\\") {
			return fmt.Errorf("недопустимое имя подписки %q", sub.Name)
		}
		if seen[sub.Name] {
			return fmt.Errorf("две подписки с именем %q", sub.Name)
		}
		seen[sub.Name] = true
		if !strings.HasPrefix(sub.URL, "http://") && !strings.HasPrefix(sub.URL, "https://") {
			return fmt.Errorf("подписка %q: ссылка должна начинаться с https://", sub.Name)
		}
	}
	for _, site := range s.Sites {
		if !groupName.MatchString(site.Name) {
			return fmt.Errorf("недопустимое название сайта %q", site.Name)
		}
		if len(site.Domains) == 0 && len(site.IPs) == 0 && len(site.Ports) == 0 {
			return fmt.Errorf("у «%s» нет ни одного домена, IP или порта", site.Name)
		}
		for _, port := range site.Ports {
			if !portRe.MatchString(strings.TrimSpace(port)) {
				return fmt.Errorf("«%s»: неверный порт %q", site.Name, port)
			}
		}
		for _, d := range site.Domains {
			if normDomain(d) == "" {
				return fmt.Errorf("«%s»: неверный домен %q", site.Name, d)
			}
		}
		for _, ip := range site.IPs {
			if _, err := parseCIDR(ip); err != nil {
				return fmt.Errorf("«%s»: неверный IP %q", site.Name, ip)
			}
		}
	}
	for _, d := range s.Devices {
		if _, err := netip.ParseAddr(d.IP); err != nil {
			return fmt.Errorf("устройство «%s»: неверный IP %q", d.Name, d.IP)
		}
	}
	return nil
}

var portRe = regexp.MustCompile(`^\d{1,5}(-\d{1,5})?$`)

var domainRe = regexp.MustCompile(`^(\*\.)?([a-z0-9-]+\.)*[a-z0-9-]+$`)

// normDomain accepts "https://Example.com/path", "*.example.com" or "example.com".
func normDomain(d string) string {
	d = strings.ToLower(strings.TrimSpace(d))
	d = strings.TrimPrefix(strings.TrimPrefix(d, "https://"), "http://")
	d = strings.TrimPrefix(d, "www.")
	if i := strings.IndexAny(d, "/:?#"); i >= 0 {
		d = d[:i]
	}
	if !domainRe.MatchString(d) || !strings.Contains(d, ".") && d != "local" {
		return ""
	}
	return d
}

func parseCIDR(s string) (netip.Prefix, error) {
	s = strings.TrimSpace(s)
	if p, err := netip.ParsePrefix(s); err == nil {
		return p, nil
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Prefix{}, err
	}
	return netip.PrefixFrom(a, a.BitLen()), nil
}

// Build generates the full config.yaml.
func Build(s Settings) ([]byte, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	enabled := map[string]bool{}
	for _, id := range s.Services {
		if _, ok := ServiceByID(id); !ok {
			return nil, fmt.Errorf("неизвестный сервис %q", id)
		}
		enabled[id] = true
	}

	cfg := M{
		// Both inbounds put XKeen in Hybrid mode (its own default): TCP comes in through
		// REDIRECT, UDP through TPROXY. With TPROXY alone, KeeneticOS 5 drops TLS packets
		// delivered locally on port 443 (its web-interface hook), so no HTTPS site opened
		// from the home network while the router itself was fine.
		{"listeners", []M{
			{{"name", "tproxy"}, {"type", "tproxy"}, {"port", TProxyPort}},
			{{"name", "redir"}, {"type", "redir"}, {"port", RedirPort}},
		}},
		{"mixed-port", 1080},
		{"tcp-concurrent", true},
		{"allow-lan", true},
		{"unified-delay", true},
		{"mode", "rule"},
		{"log-level", "silent"},
		{"ipv6", false},
		{"external-controller", "0.0.0.0:9090"},
	}
	if s.Secret != "" {
		cfg = append(cfg, KV{"secret", s.Secret})
	}
	cfg = append(cfg,
		KV{"external-ui", "./zash"},
		KV{"external-ui-url", "https://github.com/Zephyruso/zashboard/releases/latest/download/dist-cdn-fonts.zip"},
		KV{"profile", M{{"store-selected", true}}},
		KV{"find-process-mode", "always"},
		KV{"sniffer", M{
			{"enable", true}, {"force-dns-mapping", true}, {"parse-pure-ip", true}, {"override-destination", false},
			{"sniff", M{
				{"HTTP", M{{"ports", []int{80}}, {"override-destination", true}}},
				{"TLS", M{{"ports", []int{443}}}},
				{"QUIC", M{{"ports", []int{443}}}},
			}},
		}},
	)
	if len(s.Proxies) > 0 {
		var proxies []M
		names := map[string]int{}
		for _, p := range s.Proxies {
			m := M{}
			for _, k := range p.Keys {
				v := p.Values[k]
				if k == "name" {
					v = uniqueName(names, fmt.Sprint(v))
				}
				m = append(m, KV{k, v})
			}
			proxies = append(proxies, m)
		}
		cfg = append(cfg, KV{"proxies", proxies})
	}
	groups, auto := mainGroups(s.Mode, s.excludeFilter())
	viaVPN := func(id string, def bool) bool {
		if v, ok := s.ServiceVPN[id]; ok {
			return v
		}
		return def
	}
	choices := func(vpnFirst bool) []string {
		if vpnFirst {
			return []string{GroupVPN, auto, "DIRECT"}
		}
		return []string{"DIRECT", GroupVPN, auto}
	}
	for _, id := range groupDisplayOrder {
		if !enabled[id] {
			continue
		}
		svc, _ := ServiceByID(id)
		groups = append(groups, serviceGroup(svc.Name, svc.Icon, choices(viaVPN(id, svc.ViaVPN))))
	}
	for _, site := range s.Sites {
		if site.Policy == PolicyGroup && site.Target == "" {
			groups = append(groups, serviceGroup(site.Name, site.Icon, choices(true)))
		}
	}
	groups = append(groups, serviceGroup(GroupOther, iconBase+"Global.png", []string{"DIRECT", auto, GroupVPN}))
	// mihomo keeps groups and providers in one namespace: a subscription called
	// "VPN" would clash with the VPN group, so such providers get another name.
	reserved := map[string]bool{"DIRECT": true, "REJECT": true, "GLOBAL": true, "PASS": true}
	for _, g := range groups {
		for _, kv := range g {
			if kv.K == "name" {
				reserved[fmt.Sprint(kv.V)] = true
			}
		}
	}
	if len(s.Subscriptions) > 0 {
		providers := M{}
		for _, sub := range s.Subscriptions {
			pname := ProviderName(sub.Name, reserved)
			p := M{{"type", "http"}, {"url", sub.URL}, {"path", "./proxy-providers/" + pname + ".yaml"}, {"interval", orInt(sub.Interval, 3600)}}
			if len(sub.Headers) > 0 {
				h := M{}
				for _, k := range sortedKeys(sub.Headers) {
					h = append(h, KV{k, []string{sub.Headers[k]}})
				}
				p = append(p, KV{"header", h})
			}
			p = append(p, KV{"health-check", M{{"enable", true}, {"url", "http://www.msftncsi.com/ncsi.txt"}, {"interval", 3000}}})
			providers = append(providers, KV{pname, p})
		}
		cfg = append(cfg, KV{"proxy-providers", providers})
	}

	cfg = append(cfg, KV{"proxy-groups", groups})

	rules, used := buildRules(s, enabled, auto)
	rp := M{}
	for _, id := range used {
		rp = append(rp, KV{id, providerYAML(Providers[id])})
	}
	cfg = append(cfg, KV{"rule-providers", rp}, KV{"rules", rules})

	return yaml.Marshal(cfg)
}

func orInt(v, d int) int {
	if v <= 0 {
		return d
	}
	return v
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func uniqueName(seen map[string]int, name string) string {
	seen[name]++
	if n := seen[name]; n > 1 {
		return fmt.Sprintf("%s (%d)", name, n)
	}
	return name
}

// mainGroups returns the top-level selection groups and the name of the automatic one.
// Simple: VPN + ⚡️ Fastest (rockblack "Простой"). Extended: Auto-Best + Fallback + Manual.
func mainGroups(mode, exclude string) ([]M, string) {
	const testURL = "https://www.gstatic.com/generate_204"
	auto := func(m M) M {
		if exclude == "" {
			return m
		}
		out := M{}
		for _, kv := range m {
			out = append(out, kv)
			if kv.K == "include-all" {
				out = append(out, KV{"exclude-filter", exclude})
			}
		}
		return out
	}
	if mode == "extended" {
		return []M{
			{{"name", GroupVPN}, {"type", "select"}, {"icon", iconBase + "Hijacking.png"}, {"proxies", []string{GroupAutoBest, GroupFallback, GroupManual}}},
			auto(M{{"name", GroupAutoBest}, {"type", "url-test"}, {"hidden", true}, {"icon", iconBase + "Auto.png"}, {"include-all", true}, {"url", testURL}, {"interval", 300}, {"tolerance", autoTolerance}}),
			auto(M{{"name", GroupFallback}, {"type", "fallback"}, {"hidden", true}, {"icon", iconBase + "Available.png"}, {"include-all", true}, {"url", testURL}, {"interval", 120}}),
			{{"name", GroupManual}, {"type", "select"}, {"icon", iconBase + "Static.png"}, {"include-all", true}},
		}, GroupAutoBest
	}
	return []M{
		{{"name", GroupVPN}, {"type", "select"}, {"icon", iconBase + "Hijacking.png"}, {"include-all", true}, {"proxies", []string{GroupFastest}}},
		auto(M{{"name", GroupFastest}, {"type", "url-test"}, {"hidden", true}, {"icon", iconBase + "Auto.png"}, {"include-all", true}, {"url", testURL}, {"interval", 300}, {"tolerance", autoTolerance}}),
	}, GroupFastest
}

func serviceGroup(name, icon string, proxies []string) M {
	g := M{{"name", name}, {"type", "select"}}
	if icon != "" {
		g = append(g, KV{"icon", icon})
	}
	return append(g, KV{"include-all", true}, KV{"proxies", proxies})
}

func providerYAML(p RuleProvider) M {
	if p.Payload != nil {
		return M{{"type", "inline"}, {"behavior", p.Behavior}, {"payload", p.Payload}}
	}
	return M{{"type", "http"}, {"behavior", p.Behavior}, {"format", p.Format}, {"interval", 86400}, {"url", p.URL}, {"path", p.Path}}
}

// buildRules orders rules the way rockblack does: local and ads first, then the user's
// device and site overrides, then services, then Russian traffic direct, then the rest.
func buildRules(s Settings, enabled map[string]bool, auto string) ([]string, []string) {
	var rules, used []string
	usedSet := map[string]bool{}
	ruleSet := func(id, target string, noResolve bool) {
		r := "RULE-SET," + id + "," + target
		if noResolve {
			r += ",no-resolve"
		}
		rules = append(rules, r)
		if !usedSet[id] {
			usedSet[id] = true
			used = append(used, id)
		}
	}
	policyTarget := func(p Policy) string {
		switch p {
		case PolicyDirect:
			return "DIRECT"
		case PolicyBlock:
			return "REJECT"
		default:
			return GroupVPN
		}
	}

	ruleSet("geosite-private", "DIRECT", false)
	ruleSet("geoip-private", "DIRECT", true)
	rules = append(rules, "DOMAIN-SUFFIX,local,DIRECT")
	for _, d := range s.Devices {
		cidr, _ := parseCIDR(d.IP)
		rules = append(rules, "SRC-IP-CIDR,"+cidr.String()+","+policyTarget(d.Policy))
	}
	if s.BlockAds {
		ruleSet("ads_domains", "REJECT", false)
	}
	ruleSet("apple", "DIRECT", false)
	// GitHub is no longer pinned to DIRECT here: it belongs to the "Разработка" tile, and the
	// tile's VPN/direct choice must win. (Rule lists are downloaded by mihomo itself, which
	// does not go through these rules.)

	// Own sites sit above the catalog so they always win.
	serviceName := map[string]string{}
	for _, svc := range Services {
		serviceName[svc.ID] = svc.Name
	}
	for _, site := range s.Sites {
		target := policyTarget(site.Policy)
		if site.Policy == PolicyGroup {
			target = site.Name
		}
		if site.Target != "" && enabled[site.Target] {
			target = serviceName[site.Target]
		}
		for _, d := range site.Domains {
			d = normDomain(d)
			if strings.HasPrefix(d, "*.") {
				d = d[2:]
			}
			rules = append(rules, "DOMAIN-SUFFIX,"+d+","+target)
		}
		for _, ip := range site.IPs {
			cidr, _ := parseCIDR(ip)
			rules = append(rules, "IP-CIDR,"+cidr.String()+","+target+",no-resolve")
		}
		for _, port := range site.Ports {
			rules = append(rules, "DST-PORT,"+strings.TrimSpace(port)+","+target)
		}
	}

	byOrder := append([]Service(nil), Services...)
	sort.SliceStable(byOrder, func(i, j int) bool { return byOrder[i].Order < byOrder[j].Order })
	for _, svc := range byOrder {
		if !enabled[svc.ID] {
			continue
		}
		noRes := map[string]bool{}
		for _, id := range svc.NoResolve {
			noRes[id] = true
		}
		for _, r := range svc.Rules {
			if id, ok := strings.CutPrefix(r, "RULE-SET,"); ok {
				ruleSet(id, svc.Name, noRes[id])
			} else {
				rules = append(rules, r+","+svc.Name)
			}
		}
	}
	ruleSet("geosite-ru", "DIRECT", false)
	ruleSet("geoip-ru", "DIRECT", true)
	rules = append(rules, "MATCH,"+GroupOther)
	return rules, used
}

// ProviderName returns the proxy-provider name for a subscription, avoiding
// names already taken by proxy groups.
func ProviderName(name string, reserved map[string]bool) string {
	out := name
	for i := 1; reserved[out]; i++ {
		out = name + " подписка"
		if i > 1 {
			out = fmt.Sprintf("%s подписка %d", name, i)
		}
	}
	return out
}
