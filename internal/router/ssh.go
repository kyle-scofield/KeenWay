// Package router talks to a Keenetic router with XKeen over SSH: runs shell scripts,
// Keenetic CLI commands, and applies mihomo configs safely.
package router

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// Target is how to reach the router.
type Target struct {
	Host     string `json:"host"`
	User     string `json:"user"`
	Password string `json:"-"`
	// HostKeys remembers each port's server key fingerprint after the first connect (trust on first use).
	HostKeys map[string]string `json:"hostKeys,omitempty"`
}

// Conn is an open SSH connection. Entware's own SSH (port 222) gives a shell directly;
// Keenetic's SSH (port 22) gives the router CLI, where "exec sh" opens the Entware shell.
type Conn struct {
	client *ssh.Client
	local  bool // running on the router itself: scripts go to a local sh
	// runHook, when set (tests), receives scripts instead of running them.
	runHook func(script string) string
	Port    int
	CLI     bool // connected to the Keenetic CLI rather than a plain shell
	// NewHostKey is set when this connection recorded a host key for the first time.
	NewHostKey string
}

// ErrHostKeyChanged means the router's SSH key differs from the remembered one (possible spoofing or reset).
var ErrHostKeyChanged = errors.New("ключ SSH роутера изменился — если роутер не сбрасывали, это может быть подмена")

func fingerprint(k ssh.PublicKey) string {
	sum := sha256.Sum256(k.Marshal())
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])
}

// Connect tries Entware SSH on port 222 first and falls back to the Keenetic CLI on port 22.
func Connect(ctx context.Context, t Target) (*Conn, error) {
	var errs []string
	for _, attempt := range []struct {
		port int
		user string
		cli  bool
	}{{222, "root", false}, {22, orDefault(t.User, "admin"), true}} {
		c, err := dial(ctx, t, attempt.port, attempt.user)
		if err == nil {
			c.CLI = attempt.cli
			return c, nil
		}
		if errors.Is(err, ErrHostKeyChanged) {
			return nil, err
		}
		errs = append(errs, fmt.Sprintf("порт %d: %v", attempt.port, err))
	}
	return nil, fmt.Errorf("не удалось подключиться к роутеру %s (%s)", t.Host, strings.Join(errs, "; "))
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func dial(ctx context.Context, t Target, port int, user string) (*Conn, error) {
	conn := &Conn{Port: port}
	key := fmt.Sprint(port)
	cfg := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(t.Password),
			ssh.KeyboardInteractive(func(_, _ string, questions []string, _ []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for i := range answers {
					answers[i] = t.Password
				}
				return answers, nil
			}),
		},
		HostKeyCallback: func(_ string, _ net.Addr, k ssh.PublicKey) error {
			fp := fingerprint(k)
			if known := t.HostKeys[key]; known != "" && known != fp {
				return ErrHostKeyChanged
			} else if known == "" {
				conn.NewHostKey = fp
			}
			return nil
		},
		Timeout: 8 * time.Second,
	}
	d := net.Dialer{Timeout: 8 * time.Second}
	nc, err := d.DialContext(ctx, "tcp", net.JoinHostPort(t.Host, key))
	if err != nil {
		return nil, fmt.Errorf("нет соединения")
	}
	sc, chans, reqs, err := ssh.NewClientConn(nc, net.JoinHostPort(t.Host, key), cfg)
	if err != nil {
		nc.Close()
		if errors.Is(err, ErrHostKeyChanged) || strings.Contains(err.Error(), ErrHostKeyChanged.Error()) {
			return nil, ErrHostKeyChanged
		}
		if strings.Contains(err.Error(), "unable to authenticate") {
			return nil, fmt.Errorf("неверный логин или пароль")
		}
		return nil, err
	}
	conn.client = ssh.NewClient(sc, chans, reqs)
	return conn, nil
}

// Local returns a connection for KeenWay running on the router itself.
func Local() *Conn { return &Conn{local: true} }

// IsLocal reports whether commands run on this machine.
func (c *Conn) IsLocal() bool { return c.local }

func (c *Conn) Close() error {
	if c.local {
		return nil
	}
	return c.client.Close()
}

// Shell runs a POSIX shell script in the Entware environment and returns its stdout.
func (c *Conn) Shell(ctx context.Context, script string) (string, error) {
	if c.runHook != nil {
		return c.runHook(script), nil
	}
	cmd := "sh"
	if c.CLI {
		cmd = "exec sh"
	}
	out, err := c.run(ctx, cmd, script)
	if c.CLI {
		out = cleanCLI(out)
	}
	return out, err
}

// Command runs one Keenetic CLI command ("show ip hotspot"); over Entware SSH it goes through ndmc.
func (c *Conn) Command(ctx context.Context, cmd string) (string, error) {
	if c.CLI {
		out, err := c.run(ctx, cmd, "")
		return cleanCLI(out), err
	}
	return c.run(ctx, "ndmc -c "+shellQuote(cmd), "")
}

func (c *Conn) run(ctx context.Context, cmd, stdin string) (string, error) {
	if c.local {
		return runLocal(ctx, cmd, stdin)
	}
	s, err := c.client.NewSession()
	if err != nil {
		return "", err
	}
	defer s.Close()
	var stdout, stderr bytes.Buffer
	s.Stdout, s.Stderr = &stdout, &stderr
	if stdin != "" {
		// Every script ends with an explicit exit so the Keenetic "exec sh" session closes.
		s.Stdin = strings.NewReader(stdin + "\nexit\n")
	}
	done := make(chan error, 1)
	go func() { done <- s.Run(cmd) }()
	select {
	case err = <-done:
	case <-ctx.Done():
		s.Signal(ssh.SIGKILL)
		return stdout.String(), fmt.Errorf("роутер не ответил вовремя")
	}
	if err != nil {
		var exit *ssh.ExitError
		if errors.As(err, &exit) {
			return stdout.String(), fmt.Errorf("команда завершилась с ошибкой %d: %s", exit.ExitStatus(), strings.TrimSpace(stderr.String()+" "+lastLines(stdout.String(), 3)))
		}
		if _, ok := err.(*ssh.ExitMissingError); !ok {
			return stdout.String(), err
		}
	}
	return stdout.String(), nil
}

// cleanCLI strips terminal escape codes the Keenetic CLI adds to its output.
func cleanCLI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && (s[j] < '@' || s[j] > '~') {
				j++
			}
			i = j
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " | ")
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
