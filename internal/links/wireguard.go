package links

import (
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
)

var wgBlockStart = regexp.MustCompile(`(?m)^\s*\[Interface\]\s*$`)

// splitWireGuard returns each "[Interface] … [Peer] …" config found in the text.
func splitWireGuard(text string) []string {
	idx := wgBlockStart.FindAllStringIndex(text, -1)
	var blocks []string
	for i, loc := range idx {
		end := len(text)
		if i+1 < len(idx) {
			end = idx[i+1][0]
		}
		block := text[loc[0]:end]
		// A config ends at the first line that isn't part of an INI section.
		var kept []string
		for _, line := range strings.Split(block, "\n") {
			t := strings.TrimSpace(line)
			if shareScheme.MatchString(t) || strings.HasPrefix(strings.ToLower(t), "http") {
				break
			}
			kept = append(kept, line)
		}
		blocks = append(blocks, strings.Join(kept, "\n"))
	}
	return blocks
}

func removeWireGuard(text string) string {
	for _, b := range splitWireGuard(text) {
		text = strings.Replace(text, b, "", 1)
	}
	return text
}

// amneziaKeys are the obfuscation parameters AmneziaWG 1.0–2.0 adds to a WireGuard config.
var amneziaKeys = []string{"jc", "jmin", "jmax", "s1", "s2", "s3", "s4", "h1", "h2", "h3", "h4", "i1", "i2", "i3", "i4", "i5", "j1", "j2", "j3", "itime"}

// ParseWireGuard converts a WireGuard or AmneziaWG config into a mihomo proxy.
func ParseWireGuard(conf string) (*Proxy, error) {
	section := ""
	iface := map[string]string{}
	peer := map[string]string{}
	name := ""
	for _, line := range strings.Split(conf, "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if strings.HasPrefix(t, "#") {
			// "# Name = Home" style comments are common in exported configs.
			if k, v, ok := strings.Cut(strings.TrimLeft(t, "# "), "="); ok && strings.EqualFold(strings.TrimSpace(k), "name") {
				name = strings.TrimSpace(v)
			}
			continue
		}
		if strings.HasPrefix(t, "[") {
			section = strings.ToLower(strings.Trim(t, "[]"))
			continue
		}
		k, v, ok := strings.Cut(t, "=")
		if !ok {
			continue
		}
		k, v = strings.ToLower(strings.TrimSpace(k)), strings.TrimSpace(v)
		switch section {
		case "interface":
			iface[k] = v
		case "peer":
			if _, seen := peer[k]; !seen { // only the first peer is used
				peer[k] = v
			}
		}
	}
	if iface["privatekey"] == "" || peer["publickey"] == "" || peer["endpoint"] == "" {
		return nil, fmt.Errorf("WireGuard: нужны PrivateKey, PublicKey и Endpoint")
	}
	host, portStr, err := net.SplitHostPort(peer["endpoint"])
	if err != nil {
		return nil, fmt.Errorf("WireGuard: неверный Endpoint %q", peer["endpoint"])
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("WireGuard: неверный порт в Endpoint")
	}

	p := newProxy()
	isAmnezia := false
	for _, k := range amneziaKeys {
		if iface[k] != "" {
			isAmnezia = true
		}
	}
	if name == "" {
		if isAmnezia {
			name = "AmneziaWG " + host
		} else {
			name = "WireGuard " + host
		}
	}
	p.Set("name", cleanName(name))
	p.Set("type", "wireguard")
	p.Set("server", host)
	p.Set("port", port)
	for _, a := range strings.Split(iface["address"], ",") {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(a))
		addr := prefix.Addr()
		if err != nil {
			if addr, err = netip.ParseAddr(strings.TrimSpace(a)); err != nil {
				continue
			}
		}
		if addr.Is4() {
			p.Set("ip", addr.String())
		} else {
			p.Set("ipv6", addr.String())
		}
	}
	p.Set("private-key", iface["privatekey"])
	p.Set("public-key", peer["publickey"])
	p.SetIf("pre-shared-key", peer["presharedkey"])
	if mtu, err := strconv.Atoi(iface["mtu"]); err == nil {
		p.Set("mtu", mtu)
	}
	if allowed := peer["allowedips"]; allowed != "" {
		var list []string
		for _, a := range strings.Split(allowed, ",") {
			if a = strings.TrimSpace(a); a != "" {
				list = append(list, a)
			}
		}
		p.Set("allowed-ips", list)
	} else {
		p.Set("allowed-ips", []string{"0.0.0.0/0"})
	}
	p.Set("udp", true)
	if isAmnezia {
		opts := map[string]any{}
		for _, k := range amneziaKeys {
			v := iface[k]
			if v == "" {
				continue
			}
			// Numbers stay numbers; AWG 2.0 ranges ("100-200") and packet templates stay strings.
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				opts[k] = n
			} else {
				opts[k] = v
			}
		}
		p.Set("amnezia-wg-option", opts)
	}
	return p, nil
}
