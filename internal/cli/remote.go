package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"

	"keenway/internal/core"
	"keenway/internal/router"
	"keenway/internal/store"
)

// NewRemote is the terminal program on a computer: the same menu and commands,
// but the router is reached over SSH and its mihomo API over the network.
// Settings are shared with the window version.
func NewRemote(st *store.Store, version string) *App {
	a := New(st, version)
	a.remote = true
	a.E = &core.Engine{St: st, MihomoHost: st.Get().Router.Host}
	a.E.Connect = a.connectRemote
	return a
}

// connectRemote opens SSH to the router, asking for the password when none is saved.
func (a *App) connectRemote(ctx context.Context) (*router.Conn, error) {
	st := a.E.St.Get()
	t := st.Router
	t.Password = st.RouterPassword
	if t.Password == "" {
		t.Password = a.routerPW
	}
	if t.Password == "" {
		pw, err := a.askSecret(fmt.Sprintf("Пароль роутера %s (пользователь %s): ", t.Host, t.User))
		if err != nil || pw == "" {
			return nil, fmt.Errorf("нужен пароль роутера")
		}
		t.Password, a.routerPW = pw, pw
	}
	c, err := router.Connect(ctx, t)
	if err != nil {
		return nil, err
	}
	if c.NewHostKey != "" {
		a.E.St.Update(func(s *store.State) {
			if s.Router.HostKeys == nil {
				s.Router.HostKeys = map[string]string{}
			}
			s.Router.HostKeys[fmt.Sprint(c.Port)] = c.NewHostKey
		})
	}
	return c, nil
}

// askSecret reads a line without echo when stdin is a terminal.
func (a *App) askSecret(prompt string) (string, error) {
	fmt.Fprint(a.Out, prompt)
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		b, err := term.ReadPassword(fd)
		fmt.Fprintln(a.Out)
		return strings.TrimSpace(string(b)), err
	}
	line, err := a.In.ReadString('\n')
	return strings.TrimSpace(line), err
}

// onboard runs once on a computer: which router, login, password, remember it or not.
func (a *App) onboard(ctx context.Context) error {
	st := a.E.St.Get()
	if st.Imported && st.Router.Host != "" {
		return nil
	}
	a.logo()
	a.p("")
	a.p("Подключение к роутеру Keenetic с XKeen. Данные нужны один раз.")
	host := strings.TrimSpace(a.ask(fmt.Sprintf("IP роутера [%s]: ", or(st.Router.Host, "192.168.1.1"))))
	if host == "" {
		host = or(st.Router.Host, "192.168.1.1")
	}
	user := strings.TrimSpace(a.ask(fmt.Sprintf("Логин [%s]: ", or(st.Router.User, "admin"))))
	if user == "" {
		user = or(st.Router.User, "admin")
	}
	pw, err := a.askSecret("Пароль: ")
	if err != nil || pw == "" {
		return fmt.Errorf("нужен пароль роутера")
	}
	remember := a.confirm("Запомнить пароль на этом компьютере?")
	a.E.St.Update(func(s *store.State) {
		if s.Router.Host != host {
			s.Router.HostKeys = nil
		}
		s.Router.Host, s.Router.User = host, user
		s.RouterPassword = ""
		if remember {
			s.RouterPassword = pw
		}
	})
	a.routerPW = pw
	a.E.MihomoHost = host
	a.p("Подключение…")
	c, err := a.E.Connect(ctx)
	if err != nil {
		return err
	}
	c.Close()
	a.p("%s Роутер %s подключён", a.green("✔"), host)
	return nil
}

func or(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
