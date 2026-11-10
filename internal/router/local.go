package router

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// runLocal executes a script with the router's Entware shell when KeenWay runs on the router.
func runLocal(ctx context.Context, cmd, stdin string) (string, error) {
	if cmd != "sh" && cmd != "exec sh" {
		return "", fmt.Errorf("команды Keenetic CLI недоступны в режиме роутера")
	}
	sh := "/opt/bin/sh"
	if _, err := exec.LookPath(sh); err != nil {
		sh = "sh"
	}
	c := exec.CommandContext(ctx, sh)
	c.Env = append(c.Environ(), "PATH=/opt/bin:/opt/sbin:/sbin:/bin:/usr/sbin:/usr/bin")
	c.Stdin = strings.NewReader(stdin + "\nexit\n")
	var stdout, stderr bytes.Buffer
	c.Stdout, c.Stderr = &stdout, &stderr
	err := c.Run()
	if ctx.Err() != nil {
		return stdout.String(), fmt.Errorf("роутер не ответил вовремя")
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return stdout.String(), fmt.Errorf("команда завершилась с ошибкой %d: %s", exit.ExitCode(), strings.TrimSpace(stderr.String()+" "+lastLines(stdout.String(), 3)))
	}
	return stdout.String(), err
}
