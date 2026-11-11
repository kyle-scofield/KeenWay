package gen

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"keenway/internal/links"
)

// Import reads an existing mihomo config (e.g. from the rockblack generator) back into
// Settings, so the user's subscriptions, services and hand-added rules survive.
// Anything it can't map is returned as a warning rather than silently dropped.
// droppedTemplateRules are template rules KeenWay intentionally no longer generates.
var droppedTemplateRules = []string{"DOMAIN-SUFFIX,github.com,DIRECT", "DOMAIN-SUFFIX,githubusercontent.com,DIRECT", "DOMAIN-SUFFIX,raw.githubusercontent.com,DIRECT"}

func Import(config []byte) (Settings, []string, error) {
	var doc struct {
		Secret         string                    `yaml:"secret"`
		Proxies        []yaml.Node               `yaml:"proxies"`
		ProxyProviders map[string]map[string]any `yaml:"proxy-providers"`
		ProxyGroups    []map[string]any          `yaml:"proxy-groups"`
		Rules          []string                  `yaml:"rules"`
	}
	if err := yaml.Unmarshal(config, &doc); err != nil {
		return Settings{}, nil, fmt.Errorf("не удалось прочитать конфиг: %v", err)
	}
	s := Settings{Mode: "simple", Secret: doc.Secret, ServiceVPN: map[string]bool{}}
	var warnings []string

	for name, p := range doc.ProxyProviders {
		url, _ := p["url"].(string)
		if url == "" {
			warnings = append(warnings, "подписка "+name+" без ссылки пропущена")
			continue
		}
		sub := Subscription{Name: strings.TrimSpace(name), URL: url, Interval: toInt(p["interval"])}
		if h, ok := p["header"].(map[string]any); ok {
			sub.Headers = map[string]string{}
			for k, v := range h {
				if list, ok := v.([]any); ok && len(list) > 0 {
					sub.Headers[k] = fmt.Sprint(list[0])
				} else if v != nil {
					sub.Headers[k] = fmt.Sprint(v)
				}
			}
		}
		s.Subscriptions = append(s.Subscriptions, sub)
	}
	sortSubs(s.Subscriptions)

	for _, n := range doc.Proxies {
		p, err := proxyFromNode(&n)
		if err != nil {
			warnings = append(warnings, err.Error())
			continue
		}
		s.Proxies = append(s.Proxies, p)
	}

	byName := map[string]Service{}
	for _, svc := range Services {
		byName[svc.Name] = svc
	}
	for _, g := range doc.ProxyGroups {
		name, _ := g["name"].(string)
		if name == GroupAutoBest || name == GroupFallback {
			s.Mode = "extended"
		}
		if name == GroupAutoBest || name == GroupFastest {
			if f, ok := g["exclude-filter"].(string); ok {
				s.AutoExclude = []string{}
				for _, w := range strings.Split(f, "|") {
					if w = regexpUnquote.ReplaceAllString(w, "$1"); w != "" {
						s.AutoExclude = append(s.AutoExclude, w)
					}
				}
			} else {
				s.AutoExclude = []string{}
			}
		}
		svc, ok := byName[name]
		if !ok {
			continue
		}
		s.Services = append(s.Services, svc.ID)
		if first := firstProxy(g["proxies"]); first != "" {
			if vpn := first != "DIRECT"; vpn != svc.ViaVPN {
				s.ServiceVPN[svc.ID] = vpn
			}
		}
	}
	s.Services = catalogOrder(s.Services)

	// Rules the catalog produces itself are recognised and skipped; the rest become the user's own sites.
	catalogRules := map[string]bool{}
	if gen, err := Build(Settings{Mode: s.Mode, Subscriptions: []Subscription{{Name: "x", URL: "https://x"}}, Services: s.Services, BlockAds: true}); err == nil {
		var d struct {
			Rules []string `yaml:"rules"`
		}
		yaml.Unmarshal(gen, &d)
		for _, r := range d.Rules {
			catalogRules[r] = true
		}
	}
	// The rockblack template pins GitHub to DIRECT; KeenWay leaves GitHub to the
	// "Разработка" tile, so these are dropped instead of becoming the user's own sites.
	for _, r := range droppedTemplateRules {
		catalogRules[r] = true
	}
	sites := map[string]*CustomSite{}
	var order []string
	site := func(target string) *CustomSite {
		if cs, ok := sites[target]; ok {
			return cs
		}
		cs := &CustomSite{}
		switch {
		case target == "DIRECT":
			cs.Name, cs.Policy = "Сайты напрямую", PolicyDirect
		case target == "REJECT":
			cs.Name, cs.Policy = "Заблокированные сайты", PolicyBlock
		case target == GroupVPN:
			cs.Name, cs.Policy = "Сайты через VPN", PolicyVPN
		case byName[target].ID != "":
			cs.Name, cs.Target = SiteTitleFor(byName[target].ID), byName[target].ID
		default:
			cs.Name, cs.Policy = target, PolicyGroup
		}
		sites[target] = cs
		order = append(order, target)
		return cs
	}
	for _, r := range doc.Rules {
		if catalogRules[r] {
			if strings.HasPrefix(r, "RULE-SET,ads_domains,") {
				s.BlockAds = true
			}
			continue
		}
		parts := strings.Split(r, ",")
		if len(parts) < 3 {
			continue
		}
		kind, value, target := parts[0], parts[1], parts[2]
		switch kind {
		case "DOMAIN-SUFFIX", "DOMAIN":
			if value == "local" || strings.Contains(value, "github") {
				continue
			}
			cs := site(target)
			cs.Domains = append(cs.Domains, value)
		case "IP-CIDR", "IP-CIDR6":
			cs := site(target)
			cs.IPs = append(cs.IPs, value)
		case "DST-PORT":
			cs := site(target)
			cs.Ports = append(cs.Ports, value)
		case "SRC-IP-CIDR":
			p := PolicyVPN
			if target == "DIRECT" {
				p = PolicyDirect
			} else if target == "REJECT" {
				p = PolicyBlock
			}
			s.Devices = append(s.Devices, DeviceRule{Name: value, IP: strings.SplitN(value, "/", 2)[0], Policy: p})
		case "RULE-SET":
			if value == "ads_domains" {
				s.BlockAds = true
				continue
			}
			warnings = append(warnings, "правило не перенесено: "+r)
		default:
			warnings = append(warnings, "правило не перенесено: "+r)
		}
	}
	for _, t := range order {
		cs := sites[t]
		if cs.Policy == PolicyGroup {
			// Keep the icon of a custom group the user already had.
			for _, g := range doc.ProxyGroups {
				if g["name"] == t {
					cs.Icon, _ = g["icon"].(string)
				}
			}
		}
		s.Sites = append(s.Sites, *cs)
	}
	return s, warnings, nil
}

// ProxyFromYAML parses one mihomo proxy definition (as stored by the app after an import).
func ProxyFromYAML(text string) (*links.Proxy, error) {
	var n yaml.Node
	if err := yaml.Unmarshal([]byte(text), &n); err != nil || len(n.Content) == 0 {
		return nil, fmt.Errorf("не удалось прочитать сервер: %v", err)
	}
	return proxyFromNode(n.Content[0])
}

// ProxyToYAML is the inverse of ProxyFromYAML.
func ProxyToYAML(p *links.Proxy) string {
	m := M{}
	for _, k := range p.Keys {
		m = append(m, KV{k, p.Values[k]})
	}
	b, _ := yaml.Marshal(m)
	return string(b)
}

func proxyFromNode(n *yaml.Node) (*links.Proxy, error) {
	if n.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("сервер в конфиге записан в неизвестном формате")
	}
	p := &links.Proxy{Values: map[string]any{}}
	for i := 0; i+1 < len(n.Content); i += 2 {
		var v any
		if err := n.Content[i+1].Decode(&v); err != nil {
			return nil, err
		}
		p.Set(n.Content[i].Value, v)
	}
	if p.Name() == "" {
		return nil, fmt.Errorf("сервер без имени пропущен")
	}
	return p, nil
}

func firstProxy(v any) string {
	if l, ok := v.([]any); ok && len(l) > 0 {
		s, _ := l[0].(string)
		return s
	}
	return ""
}

func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	}
	return 0
}

func catalogOrder(ids []string) []string {
	set := map[string]bool{}
	for _, id := range ids {
		set[id] = true
	}
	var out []string
	for _, svc := range Services {
		if set[svc.ID] {
			out = append(out, svc.ID)
		}
	}
	return out
}

func sortSubs(subs []Subscription) {
	for i := 1; i < len(subs); i++ {
		for j := i; j > 0 && subs[j].Name < subs[j-1].Name; j-- {
			subs[j], subs[j-1] = subs[j-1], subs[j]
		}
	}
}

// SiteTitleFor gives an everyday name to a list of the user's own sites that
// rides along with a service group (the name is only a label in the app).
func SiteTitleFor(serviceID string) string {
	switch serviceID {
	case "blocked":
		return "Сайты через VPN"
	case "geoblock":
		return "Зарубежные сайты"
	}
	for _, svc := range Services {
		if svc.ID == serviceID {
			return "Свои сайты: " + svc.Name
		}
	}
	return "Свои сайты"
}

// TidySiteNames renames lists created by older versions ("Мои: …") and makes
// names unique. It reports whether anything changed.
func TidySiteNames(s *Settings) bool {
	changed := false
	legacy := map[string]string{"Мои сайты напрямую": "Сайты напрямую", "Мои блокировки": "Заблокированные сайты", "Мои сайты через VPN": "Сайты через VPN"}
	seen := map[string]bool{}
	for i := range s.Sites {
		cs := &s.Sites[i]
		name := cs.Name
		if rest, ok := strings.CutPrefix(name, "Мои: "); ok {
			id := cs.Target
			for _, svc := range Services {
				if id == "" && svc.Name == rest {
					id = svc.ID
				}
			}
			name = SiteTitleFor(id)
		} else if n, ok := legacy[name]; ok {
			name = n
		} else if strings.TrimSpace(name) == "" || !groupName.MatchString(name) {
			// a list saved without a usable name broke the whole config
			name = SiteTitleFor(cs.Target)
		}
		base := name
		for n := 2; seen[name]; n++ {
			name = fmt.Sprintf("%s %d", base, n)
		}
		seen[name] = true
		if name != cs.Name {
			cs.Name, changed = name, true
		}
	}
	return changed
}

var regexpUnquote = regexp.MustCompile(`\\(.)`)
