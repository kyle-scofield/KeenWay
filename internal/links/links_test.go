package links

import (
	"encoding/base64"
	"strings"
	"testing"
)

func one(t *testing.T, text string) *Proxy {
	t.Helper()
	items := Parse(text)
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d", len(items))
	}
	if items[0].Err != nil {
		t.Fatalf("parse error: %v", items[0].Err)
	}
	return items[0].Proxy
}

func TestVlessReality(t *testing.T) {
	p := one(t, "vless://11111111-2222-3333-4444-555555555555@1.2.3.4:443?type=tcp&security=reality&pbk=PUBKEY&sid=ab12&sni=www.intel.com&fp=chrome&flow=xtls-rprx-vision#%F0%9F%87%B3%F0%9F%87%B1%20NL")
	want := map[string]any{"type": "vless", "server": "1.2.3.4", "port": 443, "flow": "xtls-rprx-vision", "servername": "www.intel.com", "network": "tcp", "name": "🇳🇱 NL"}
	for k, v := range want {
		if p.Values[k] != v {
			t.Errorf("%s = %v, want %v", k, p.Values[k], v)
		}
	}
	ro := p.Values["reality-opts"].(map[string]any)
	if ro["public-key"] != "PUBKEY" || ro["short-id"] != "ab12" {
		t.Errorf("reality-opts = %v", ro)
	}
}

func TestVlessXhttp(t *testing.T) {
	p := one(t, "vless://id@h.example:20009?type=xhttp&path=%2Fabc&mode=auto&security=reality&pbk=K&sni=s#x")
	opts := p.Values["xhttp-opts"].(map[string]any)
	if p.Values["network"] != "xhttp" || opts["path"] != "/abc" || opts["mode"] != "auto" {
		t.Errorf("xhttp parse: %v %v", p.Values["network"], opts)
	}
}

func TestVmess(t *testing.T) {
	js := `{"v":"2","ps":"VM","add":"5.6.7.8","port":"8080","id":"uuid-1","aid":"0","net":"ws","path":"/ws","host":"cdn.example","tls":"tls","sni":"cdn.example"}`
	p := one(t, "vmess://"+base64.StdEncoding.EncodeToString([]byte(js)))
	if p.Values["port"] != 8080 || p.Values["network"] != "ws" || p.Values["tls"] != true || p.Name() != "VM" {
		t.Errorf("vmess parse: %v", p.Values)
	}
}

func TestTrojan(t *testing.T) {
	p := one(t, "trojan://secret@t.example:443?sni=t.example&allowInsecure=1#Tro")
	if p.Values["password"] != "secret" || p.Values["skip-cert-verify"] != true || p.Values["sni"] != "t.example" {
		t.Errorf("trojan parse: %v", p.Values)
	}
}

func TestShadowsocksForms(t *testing.T) {
	sip002 := "ss://" + base64.RawURLEncoding.EncodeToString([]byte("chacha20-ietf-poly1305:pass")) + "@9.9.9.9:8388#SS"
	legacy := "ss://" + base64.StdEncoding.EncodeToString([]byte("aes-256-gcm:pw@8.8.8.8:443")) + "#Old"
	for _, link := range []string{sip002, legacy} {
		p := one(t, link)
		if p.Values["type"] != "ss" || p.Values["cipher"] == "" || p.Values["password"] == "" {
			t.Errorf("ss parse %s: %v", link, p.Values)
		}
	}
}

func TestHysteria2(t *testing.T) {
	p := one(t, "hysteria2://authpass@203.0.113.10:20000?sni=203.0.113.10&insecure=1&obfs=salamander&obfs-password=ob#HY")
	if p.Values["type"] != "hysteria2" || p.Values["port"] != 20000 || p.Values["obfs"] != "salamander" || p.Values["skip-cert-verify"] != true {
		t.Errorf("hy2 parse: %v", p.Values)
	}
	hop := one(t, "hy2://pw@h.example:20000-21000/?sni=h.example#hop")
	if hop.Values["ports"] != "20000-21000" || hop.Values["port"] != 20000 {
		t.Errorf("hy2 port hopping: %v", hop.Values)
	}
}

func TestAmneziaWG(t *testing.T) {
	conf := `[Interface]
PrivateKey = PRIV=
Address = 10.8.0.2/32, fd00::2/128
DNS = 1.1.1.1
MTU = 1280
Jc = 4
Jmin = 40
Jmax = 70
S1 = 0
S2 = 0
H1 = 1
H2 = 2
H3 = 3
H4 = 100-200

[Peer]
PublicKey = PUB=
PresharedKey = PSK=
AllowedIPs = 0.0.0.0/0, ::/0
Endpoint = 203.0.113.5:51820
`
	p := one(t, conf)
	if p.Values["type"] != "wireguard" || p.Values["ip"] != "10.8.0.2" || p.Values["ipv6"] != "fd00::2" || p.Values["mtu"] != 1280 {
		t.Errorf("awg parse: %v", p.Values)
	}
	awg := p.Values["amnezia-wg-option"].(map[string]any)
	if awg["jc"] != int64(4) || awg["h4"] != "100-200" {
		t.Errorf("awg options: %v", awg)
	}
}

func TestMixedInputAndHapp(t *testing.T) {
	items := Parse("https://sub.example/abc\nvless://id@1.1.1.1:443?security=tls&sni=a#A\nhapp://add/https%3A%2F%2Fsub2.example%2Fx\nhapp://crypt3/AAAA\nnonsense")
	kinds := []string{}
	for _, it := range items {
		k := it.Kind
		if it.Err != nil {
			k = "err"
		}
		kinds = append(kinds, k)
	}
	if strings.Join(kinds, ",") != "subscription,proxy,subscription,err,err" {
		t.Errorf("kinds = %v", kinds)
	}
	if items[2].URL != "https://sub2.example/x" {
		t.Errorf("happ add url = %q", items[2].URL)
	}
}
