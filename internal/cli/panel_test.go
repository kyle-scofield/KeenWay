package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestPanelScriptIsValidShell(t *testing.T) {
	f := filepath.Join(t.TempDir(), "S99keenway-panel")
	os.WriteFile(f, []byte(panelScript("/opt/sbin/keenway")), 0o755)
	if out, err := exec.Command("sh", "-n", f).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
}
