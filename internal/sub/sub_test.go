package sub

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseUserInfo(t *testing.T) {
	i := ParseUserInfo("upload=1073741824; download=2147483648; total=107374182400; expire=1798761600")
	if i.Used() != 3<<30 || i.Total != 100<<30 || i.Expire.Unix() != 1798761600 {
		t.Fatalf("got %+v", i)
	}
	now := time.Unix(1798761600-10*86400-60, 0)
	if d := i.DaysLeft(now); d != 10 {
		t.Errorf("days left = %d", d)
	}
	if (Info{}).DaysLeft(now) != -1 {
		t.Errorf("no expiry should be -1")
	}
}

func TestFetchBase64WithHeaders(t *testing.T) {
	list := "vless://id@1.1.1.1:443?security=tls&sni=a#One\ntrojan://pw@2.2.2.2:443#Two\n"
	var gotHWID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHWID = r.Header.Get("x-hwid")
		w.Header().Set("subscription-userinfo", "upload=10; download=20; total=100; expire=0")
		w.Header().Set("profile-title", "base64:"+base64.StdEncoding.EncodeToString([]byte("Мой VPN")))
		w.Write([]byte(base64.StdEncoding.EncodeToString([]byte(list))))
	}))
	defer srv.Close()
	r, err := Fetch(context.Background(), srv.URL, map[string]string{"x-hwid": "ROUTERMAC"})
	if err != nil {
		t.Fatal(err)
	}
	if gotHWID != "ROUTERMAC" {
		t.Errorf("hwid header not forwarded: %q", gotHWID)
	}
	if r.Format != "base64" || len(r.Nodes) != 2 || r.Nodes[1] != "Two" {
		t.Errorf("parse: %+v", r)
	}
	if !r.HasInfo || r.Info.Used() != 30 || r.Info.Title != "Мой VPN" {
		t.Errorf("info: %+v", r.Info)
	}
}

func TestParseClashYAML(t *testing.T) {
	r := ParseBody([]byte("proxies:\n  - {name: A, type: ss, server: 1.1.1.1, port: 1, cipher: aes-128-gcm, password: x}\n  - {name: B, type: vless, server: 2.2.2.2, port: 2, uuid: u}\n"))
	if r.Format != "clash" || len(r.Nodes) != 2 {
		t.Errorf("clash parse: %+v", r)
	}
}
