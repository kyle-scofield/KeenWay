package router

import (
	"errors"
	"testing"
)

func TestCheckTestOutput(t *testing.T) {
	if err := checkTestOutput("kw_rc=0\nkw_free=30\nconfiguration file /opt/etc/mihomo/x.yaml test is successful\n"); err != nil {
		t.Fatalf("success: %v", err)
	}
	var ce *ConfigError
	err := checkTestOutput("kw_rc=1\nkw_free=30\ntime=\"x\" level=fatal msg=\"Parse config error: rules[3] [RULE-SET,nope,VPN] error: rule set [nope] not found\"\n")
	if !errors.As(err, &ce) || ce.Reason == "" {
		t.Fatalf("config error expected, got %#v", err)
	}
	var nm *ErrTestNoMemory
	err = checkTestOutput("kw_rc=137\nkw_rc=137\nkw_free=21\n")
	if !errors.As(err, &nm) || nm.FreeMB != "21" {
		t.Fatalf("no-memory expected, got %#v", err)
	}
}
