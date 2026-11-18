package core

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// LANCheck opens HTTPS sites from this computer the way any device at home does (through
// the router's transparent proxy). It returns "" when they open, or what is wrong.
func LANCheck(ctx context.Context) string {
	client := &http.Client{
		Timeout: 10 * time.Second,
		// straight out, like a phone would: no system proxy of this computer
		Transport:     &http.Transport{Proxy: nil, TLSHandshakeTimeout: 8 * time.Second},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	// this computer's own VPN (Throne, v2rayN, WireGuard…) sends traffic past the router: the check would lie
	if name := localVPN(); name != "" {
		return "на этом компьютере включён свой VPN (" + name + "), поэтому проверить путь через роутер отсюда нельзя. Проверьте с телефона в домашнем Wi-Fi."
	}
	var failed []string
	for _, u := range []string{"https://www.google.com/generate_204", "https://cp.cloudflare.com/generate_204", "https://github.com/"} {
		ok := false
		// right after a restart the router is busy testing servers: give a single site a few more tries
		for try := 0; try < 4 && !ok; try++ {
			if try > 0 {
				select {
				case <-ctx.Done():
				case <-time.After(3 * time.Second):
				}
			}
			req, _ := http.NewRequestWithContext(ctx, http.MethodHead, u, nil)
			if res, err := client.Do(req); err == nil {
				res.Body.Close()
				ok = true
			}
		}
		if !ok {
			host, _ := url.Parse(u)
			failed = append(failed, host.Host)
		}
	}
	if len(failed) == 0 {
		return ""
	}
	return "с этого компьютера не открываются " + strings.Join(failed, ", ") + ". Роутер в интернет выходит, но трафик устройств через него не проходит — откройте «Логи» → «Журналы роутера» или напишите в поддержку."
}

// localVPN: the name of an active tunnel interface of this computer's own VPN client, if any.
func localVPN() string {
	ifs, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		n := strings.ToLower(i.Name)
		if strings.Contains(n, "tun") || strings.HasPrefix(n, "wg") || strings.Contains(n, "wintun") || strings.Contains(n, "throne") ||
			strings.Contains(n, "nekoray") || strings.Contains(n, "v2ray") || strings.Contains(n, "clash") || strings.Contains(n, "amnezia") || strings.HasPrefix(n, "tap") {
			if addrs, _ := i.Addrs(); len(addrs) > 0 {
				return i.Name
			}
		}
	}
	return ""
}
