// Package links turns whatever the user pastes (share links, subscription URLs,
// WireGuard/AmneziaWG configs, Happ links) into mihomo proxies or subscriptions.
package links

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Proxy is a mihomo proxy definition; key order is kept by Keys for stable YAML output.
type Proxy struct {
	Keys   []string
	Values map[string]any
}

func newProxy() *Proxy { return &Proxy{Values: map[string]any{}} }

// Set adds or replaces a field, keeping first-insertion order.
func (p *Proxy) Set(k string, v any) {
	if _, ok := p.Values[k]; !ok {
		p.Keys = append(p.Keys, k)
	}
	p.Values[k] = v
}

// SetIf sets the field only when the value is non-empty.
func (p *Proxy) SetIf(k string, v string) {
	if v != "" {
		p.Set(k, v)
	}
}

func (p *Proxy) Name() string { s, _ := p.Values["name"].(string); return s }

// Item is one recognised entry of the user's input.
type Item struct {
	Kind  string // "proxy" or "subscription"
	Proxy *Proxy
	URL   string
	Raw   string
	Err   error
}

var shareScheme = regexp.MustCompile(`(?i)^(vless|vmess|trojan|ss|hysteria2|hy2|tuic|happ)://`)

// Parse splits pasted text into proxies and subscriptions. WireGuard configs are
// detected as whole blocks; everything else is one entry per line.
func Parse(text string) []Item {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	var items []Item
	if strings.Contains(text, "[Interface]") {
		for _, block := range splitWireGuard(text) {
			p, err := ParseWireGuard(block)
			items = append(items, Item{Kind: "proxy", Proxy: p, Raw: block, Err: err})
		}
		text = removeWireGuard(text)
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		items = append(items, ParseLine(line))
	}
	return items
}

// ParseLine recognises a single link or URL.
func ParseLine(line string) Item {
	lower := strings.ToLower(line)
	switch {
	case strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://"):
		return Item{Kind: "subscription", URL: line, Raw: line}
	case strings.HasPrefix(lower, "happ://"):
		return parseHapp(line)
	case !shareScheme.MatchString(line):
		return Item{Raw: line, Err: fmt.Errorf("не похоже на ключ или ссылку подписки")}
	}
	var p *Proxy
	var err error
	switch strings.ToLower(line[:strings.Index(line, "://")]) {
	case "vless":
		p, err = parseVless(line)
	case "vmess":
		p, err = parseVmess(line)
	case "trojan":
		p, err = parseTrojan(line)
	case "ss":
		p, err = parseShadowsocks(line)
	case "hysteria2", "hy2":
		p, err = parseHysteria2(line)
	case "tuic":
		p, err = parseTuic(line)
	}
	return Item{Kind: "proxy", Proxy: p, Raw: line, Err: err}
}

// Happ "add" links wrap a plain subscription or key; "crypt" links are encrypted
// by the provider on purpose and can't be opened without Happ's private key.
func parseHapp(line string) Item {
	rest := line[len("happ://"):]
	if strings.HasPrefix(strings.ToLower(rest), "crypt") {
		return Item{Raw: line, Err: fmt.Errorf("зашифрованная ссылка Happ (happ://crypt…) — провайдер скрыл адрес подписки; попроси у него обычную ссылку https://")}
	}
	if strings.HasPrefix(strings.ToLower(rest), "add/") {
		inner := rest[4:]
		if dec, err := url.QueryUnescape(inner); err == nil {
			inner = dec
		}
		it := ParseLine(strings.TrimSpace(inner))
		it.Raw = line
		return it
	}
	return Item{Raw: line, Err: fmt.Errorf("неизвестный формат ссылки Happ")}
}

func splitHostPort(u *url.URL) (string, int, error) {
	host := u.Hostname()
	port, err := strconv.Atoi(u.Port())
	if host == "" || err != nil || port <= 0 || port > 65535 {
		return "", 0, fmt.Errorf("не указан адрес или порт сервера")
	}
	return host, port, nil
}

func nameFrom(u *url.URL, fallback string) string {
	n := u.Fragment
	if n == "" {
		n = fallback
	}
	return cleanName(n)
}

// cleanName drops control characters and trims whitespace so names are YAML-safe and readable.
func cleanName(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
	return strings.TrimSpace(s)
}

// transport fills network-specific options shared by vless/vmess/trojan.
func transport(p *Proxy, network string, q url.Values) {
	switch network {
	case "", "tcp", "raw":
		if strings.EqualFold(q.Get("headerType"), "http") {
			p.Set("network", "http")
			opts := map[string]any{}
			if path := q.Get("path"); path != "" {
				opts["path"] = []string{path}
			}
			if host := q.Get("host"); host != "" {
				opts["headers"] = map[string]any{"Host": []string{host}}
			}
			p.Set("http-opts", opts)
			return
		}
		p.Set("network", "tcp")
	case "ws":
		p.Set("network", "ws")
		opts := map[string]any{"path": orDefault(q.Get("path"), "/")}
		if host := q.Get("host"); host != "" {
			opts["headers"] = map[string]any{"Host": host}
		}
		p.Set("ws-opts", opts)
	case "grpc":
		p.Set("network", "grpc")
		p.Set("grpc-opts", map[string]any{"grpc-service-name": q.Get("serviceName")})
	case "httpupgrade":
		p.Set("network", "ws")
		opts := map[string]any{"path": orDefault(q.Get("path"), "/"), "v2ray-http-upgrade": true}
		if host := q.Get("host"); host != "" {
			opts["headers"] = map[string]any{"Host": host}
		}
		p.Set("ws-opts", opts)
	case "xhttp", "splithttp":
		p.Set("network", "xhttp")
		opts := map[string]any{"path": orDefault(q.Get("path"), "/")}
		if host := q.Get("host"); host != "" {
			opts["host"] = host
		}
		if mode := q.Get("mode"); mode != "" {
			opts["mode"] = mode
		}
		p.Set("xhttp-opts", opts)
	case "h2", "http":
		p.Set("network", "h2")
		opts := map[string]any{"path": orDefault(q.Get("path"), "/")}
		if host := q.Get("host"); host != "" {
			opts["host"] = []string{host}
		}
		p.Set("h2-opts", opts)
	default:
		p.Set("network", network)
	}
}

func orDefault(v, d string) string {
	if v == "" {
		return d
	}
	return v
}

func truthy(v string) bool {
	v = strings.ToLower(v)
	return v == "1" || v == "true" || v == "yes"
}

func alpnList(v string) []string {
	var out []string
	for _, a := range strings.Split(v, ",") {
		if a = strings.TrimSpace(a); a != "" {
			out = append(out, a)
		}
	}
	return out
}

func parseVless(link string) (*Proxy, error) {
	u, err := url.Parse(link)
	if err != nil {
		return nil, fmt.Errorf("битая ссылка vless: %v", err)
	}
	host, port, err := splitHostPort(u)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	p := newProxy()
	p.Set("name", nameFrom(u, host))
	p.Set("type", "vless")
	p.Set("server", host)
	p.Set("port", port)
	p.Set("uuid", u.User.Username())
	p.Set("udp", true)
	transport(p, strings.ToLower(q.Get("type")), q)
	p.SetIf("flow", q.Get("flow"))
	switch strings.ToLower(q.Get("security")) {
	case "reality":
		p.Set("tls", true)
		p.SetIf("servername", q.Get("sni"))
		p.Set("client-fingerprint", orDefault(q.Get("fp"), "chrome"))
		opts := map[string]any{"public-key": q.Get("pbk")}
		if sid := q.Get("sid"); sid != "" {
			opts["short-id"] = sid
		}
		p.Set("reality-opts", opts)
	case "tls":
		p.Set("tls", true)
		p.SetIf("servername", q.Get("sni"))
		p.SetIf("client-fingerprint", q.Get("fp"))
		if a := alpnList(q.Get("alpn")); len(a) > 0 {
			p.Set("alpn", a)
		}
		if truthy(q.Get("allowInsecure")) || truthy(q.Get("insecure")) {
			p.Set("skip-cert-verify", true)
		}
	}
	if pe := q.Get("packetEncoding"); pe != "" {
		p.Set("packet-encoding", pe)
	}
	return p, nil
}

func parseVmess(link string) (*Proxy, error) {
	body := link[len("vmess://"):]
	raw, err := decodeBase64(body)
	if err != nil {
		return nil, fmt.Errorf("vmess: не удалось декодировать ссылку")
	}
	var v map[string]any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("vmess: внутри не JSON")
	}
	str := func(k string) string { return strings.TrimSpace(fmt.Sprint(orNil(v[k]))) }
	port, _ := strconv.Atoi(str("port"))
	if str("add") == "" || port == 0 {
		return nil, fmt.Errorf("vmess: нет адреса или порта")
	}
	aid, _ := strconv.Atoi(str("aid"))
	p := newProxy()
	p.Set("name", cleanName(orDefault(str("ps"), str("add"))))
	p.Set("type", "vmess")
	p.Set("server", str("add"))
	p.Set("port", port)
	p.Set("uuid", str("id"))
	p.Set("alterId", aid)
	p.Set("cipher", orDefault(str("scy"), "auto"))
	p.Set("udp", true)
	q := url.Values{}
	q.Set("path", str("path"))
	q.Set("host", str("host"))
	q.Set("headerType", str("type"))
	q.Set("serviceName", str("path"))
	transport(p, strings.ToLower(str("net")), q)
	if strings.EqualFold(str("tls"), "tls") {
		p.Set("tls", true)
		p.SetIf("servername", orDefault(str("sni"), str("host")))
		p.SetIf("client-fingerprint", str("fp"))
		if a := alpnList(str("alpn")); len(a) > 0 {
			p.Set("alpn", a)
		}
	}
	return p, nil
}

func orNil(v any) any {
	if v == nil {
		return ""
	}
	return v
}

func parseTrojan(link string) (*Proxy, error) {
	u, err := url.Parse(link)
	if err != nil {
		return nil, fmt.Errorf("битая ссылка trojan: %v", err)
	}
	host, port, err := splitHostPort(u)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	p := newProxy()
	p.Set("name", nameFrom(u, host))
	p.Set("type", "trojan")
	p.Set("server", host)
	p.Set("port", port)
	p.Set("password", u.User.Username())
	p.Set("udp", true)
	p.SetIf("sni", orDefault(q.Get("sni"), q.Get("peer")))
	p.SetIf("client-fingerprint", q.Get("fp"))
	if a := alpnList(q.Get("alpn")); len(a) > 0 {
		p.Set("alpn", a)
	}
	if truthy(q.Get("allowInsecure")) || truthy(q.Get("insecure")) {
		p.Set("skip-cert-verify", true)
	}
	if strings.EqualFold(q.Get("security"), "reality") {
		opts := map[string]any{"public-key": q.Get("pbk")}
		if sid := q.Get("sid"); sid != "" {
			opts["short-id"] = sid
		}
		p.Set("reality-opts", opts)
	}
	if n := strings.ToLower(q.Get("type")); n != "" && n != "tcp" {
		transport(p, n, q)
	}
	return p, nil
}

func parseShadowsocks(link string) (*Proxy, error) {
	body := link[len("ss://"):]
	name := ""
	if i := strings.Index(body, "#"); i >= 0 {
		name, _ = url.PathUnescape(body[i+1:])
		body = body[:i]
	}
	query := ""
	if i := strings.Index(body, "?"); i >= 0 {
		query = body[i+1:]
		body = body[:i]
	}
	body = strings.TrimSuffix(body, "/")
	var userinfo, hostport string
	if i := strings.LastIndex(body, "@"); i >= 0 {
		userinfo, hostport = body[:i], body[i+1:]
		if dec, err := decodeBase64(userinfo); err == nil && strings.Contains(string(dec), ":") {
			userinfo = string(dec)
		} else if un, err := url.PathUnescape(userinfo); err == nil {
			userinfo = un
		}
	} else {
		// Legacy form: everything base64-encoded as method:password@host:port.
		dec, err := decodeBase64(body)
		if err != nil {
			return nil, fmt.Errorf("ss: не удалось декодировать ссылку")
		}
		s := string(dec)
		i := strings.LastIndex(s, "@")
		if i < 0 {
			return nil, fmt.Errorf("ss: нет адреса сервера")
		}
		userinfo, hostport = s[:i], s[i+1:]
	}
	method, password, ok := strings.Cut(userinfo, ":")
	if !ok {
		return nil, fmt.Errorf("ss: нет метода шифрования или пароля")
	}
	host, portStr, err := net.SplitHostPort(hostport)
	if err != nil {
		return nil, fmt.Errorf("ss: неверный адрес %q", hostport)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("ss: неверный порт")
	}
	p := newProxy()
	p.Set("name", cleanName(orDefault(name, host)))
	p.Set("type", "ss")
	p.Set("server", host)
	p.Set("port", port)
	p.Set("cipher", method)
	p.Set("password", password)
	p.Set("udp", true)
	if query != "" {
		q, _ := url.ParseQuery(query)
		if plugin := q.Get("plugin"); plugin != "" {
			parts := strings.Split(plugin, ";")
			opts := map[string]any{}
			for _, kv := range parts[1:] {
				k, v, _ := strings.Cut(kv, "=")
				switch k {
				case "obfs":
					opts["mode"] = v
				case "obfs-host":
					opts["host"] = v
				case "tls":
					opts["tls"] = true
				default:
					if v == "" {
						opts[k] = true
					} else {
						opts[k] = v
					}
				}
			}
			pluginName := parts[0]
			if pluginName == "obfs-local" || pluginName == "simple-obfs" {
				pluginName = "obfs"
			}
			p.Set("plugin", pluginName)
			p.Set("plugin-opts", opts)
		}
	}
	return p, nil
}

// portRangeInAuthority matches a port-hopping range ("host:20000-21000" or "host:443,8443")
// that net/url refuses to parse.
var portRangeInAuthority = regexp.MustCompile(`^([a-zA-Z0-9]+://[^/?#]*:)(\d+[-,][\d,-]+)([/?#]|$)`)

func parseHysteria2(link string) (*Proxy, error) {
	hopping := ""
	if m := portRangeInAuthority.FindStringSubmatch(link); m != nil {
		hopping = m[2]
		first := strings.FieldsFunc(hopping, func(r rune) bool { return r == '-' || r == ',' })[0]
		link = m[1] + first + link[len(m[1])+len(m[2]):]
	}
	u, err := url.Parse(link)
	if err != nil {
		return nil, fmt.Errorf("битая ссылка hysteria2: %v", err)
	}
	host := u.Hostname()
	if host == "" {
		return nil, fmt.Errorf("hysteria2: нет адреса сервера")
	}
	q := u.Query()
	p := newProxy()
	p.Set("name", nameFrom(u, host))
	p.Set("type", "hysteria2")
	p.Set("server", host)
	if port, err := strconv.Atoi(u.Port()); err == nil {
		p.Set("port", port)
	} else {
		p.Set("port", 443)
	}
	if hopping != "" {
		p.Set("ports", hopping)
	}
	if mport := q.Get("mport"); mport != "" {
		p.Set("ports", mport)
	}
	auth := u.User.Username()
	if pw, ok := u.User.Password(); ok {
		auth += ":" + pw
	}
	p.Set("password", auth)
	p.SetIf("sni", q.Get("sni"))
	if truthy(q.Get("insecure")) || truthy(q.Get("allowInsecure")) {
		p.Set("skip-cert-verify", true)
	}
	if obfs := q.Get("obfs"); obfs != "" && obfs != "none" {
		p.Set("obfs", obfs)
		p.SetIf("obfs-password", q.Get("obfs-password"))
	}
	p.SetIf("fingerprint", q.Get("pinSHA256"))
	if a := alpnList(q.Get("alpn")); len(a) > 0 {
		p.Set("alpn", a)
	}
	return p, nil
}

func parseTuic(link string) (*Proxy, error) {
	u, err := url.Parse(link)
	if err != nil {
		return nil, fmt.Errorf("битая ссылка tuic: %v", err)
	}
	host, port, err := splitHostPort(u)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	p := newProxy()
	p.Set("name", nameFrom(u, host))
	p.Set("type", "tuic")
	p.Set("server", host)
	p.Set("port", port)
	p.Set("uuid", u.User.Username())
	pw, _ := u.User.Password()
	p.Set("password", pw)
	p.SetIf("congestion-controller", q.Get("congestion_control"))
	p.SetIf("sni", q.Get("sni"))
	if a := alpnList(q.Get("alpn")); len(a) > 0 {
		p.Set("alpn", a)
	}
	if truthy(q.Get("allow_insecure")) || truthy(q.Get("insecure")) {
		p.Set("skip-cert-verify", true)
	}
	p.Set("udp", true)
	return p, nil
}

func decodeBase64(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(s); err == nil {
			return b, nil
		}
	}
	return nil, fmt.Errorf("not base64")
}

// DecodeBase64 is exported for subscription bodies.
func DecodeBase64(s string) ([]byte, error) { return decodeBase64(s) }
