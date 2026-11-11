package core

import (
	"context"
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
	var failed []string
	for _, u := range []string{"https://www.google.com/generate_204", "https://cp.cloudflare.com/generate_204", "https://github.com/"} {
		ok := false
		for try := 0; try < 2 && !ok; try++ {
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
