package router

import (
	"strings"
	"testing"
)

func TestExplainMihomo(t *testing.T) {
	out := `time="2026-11-10T20:48:49.635590350Z" level=info msg="Start initial configuration in progress"
time="2026-11-10T20:48:49.650170055Z" level=error msg="proxy group[1]: VPN: duplicate provider name"
configuration file /opt/etc/mihomo/config.keenway-new.yaml test failed`
	got := explainMihomo(out)
	if !strings.Contains(got, "«VPN»") || !strings.Contains(got, "Переименуй") {
		t.Fatalf("got %q", got)
	}
}
