package router

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

// TestScriptsParse checks that every generated shell script is valid sh syntax.
func TestScriptsParse(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	cfg := []byte("proxy-groups:\n  - name: VPN\n  - name: \"⚡️ Fastest\"\n  - name: Сайты через VPN\n")
	var scripts []string
	c := &Conn{local: true}
	c.runHook = func(script string) string {
		scripts = append(scripts, script)
		return "configuration file test is successful\nbackup=/x\nresult=ok\n"
	}
	// Goes through upload, switch and health check; the health check "fails" and triggers a rollback.
	c.Apply(context.Background(), cfg, ApplyOptions{Secret: "a'b", NewSecret: "new"})
	c.InstallNotify(context.Background(), NotifyConfig{Token: "1:abc'd", ChatID: "42", Name: "Дом"}, true)
	c.RemoveNotify(context.Background())
	scripts = append(scripts, reloadScript("x"), reloadScript(""), notifySH)
	for _, s := range scripts {
		cmd := exec.Command("sh", "-n")
		cmd.Stdin = strings.NewReader(s)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("syntax error: %v %s\n%s", err, out, s)
		}
	}
	if len(scripts) < 6 {
		t.Fatalf("only %d scripts captured", len(scripts))
	}
}
