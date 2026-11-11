package cli

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"keenway/internal/store"
)

// DefaultPanelPort is where the router's web panel listens unless the user picks another port.
const DefaultPanelPort = 8090

// panelInit is the Entware start script of the web panel. It keeps its own pid file:
// rc.func's "killall keenway" would also kill the terminal program.
const panelInit = "/opt/etc/init.d/S99keenway-panel"

// DonateCard is shown under "Поддержать проект".
const DonateCard = "2202 2085 5093 6505"

func panelScript(bin string) string {
	return `#!/bin/sh
# KeenWay web panel (keenway panel on|off)
BIN="` + bin + `"
PID=/opt/var/run/keenway-panel.pid
LOG=/opt/var/log/keenway-panel.log
running() { [ -f "$PID" ] && kill -0 "$(cat "$PID")" 2>/dev/null; }
start() {
  running && return 0
  mkdir -p /opt/var/run /opt/var/log
  "$BIN" serve >"$LOG" 2>&1 &
  echo $! >"$PID"
}
stop() {
  if [ -f "$PID" ]; then kill "$(cat "$PID")" 2>/dev/null; rm -f "$PID"; fi
}
case "$1" in
  start) start ;;
  stop) stop ;;
  restart) stop; sleep 1; start ;;
  status) if running; then echo alive; else echo dead; fi ;;
  *) echo "usage: $0 start|stop|restart|status" ;;
esac
`
}

func (a *App) panel(ctx context.Context, args []string) error {
	if a.remote {
		return fmt.Errorf("веб-панель ставится на сам роутер: зайдите на него по SSH и выполните keenway panel on")
	}
	sub := "status"
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "on", "install":
		port := a.E.St.Get().Panel.Port
		if len(args) > 1 {
			p, err := strconv.Atoi(args[1])
			if err != nil || p < 1 || p > 65535 {
				return fmt.Errorf("порт — число от 1 до 65535")
			}
			port = p
		}
		if port == 0 {
			port = DefaultPanelPort
		}
		if busy(port) && !panelRunning() {
			return fmt.Errorf("порт %d занят другой программой; выберите другой: keenway panel on 8091", port)
		}
		if a.E.St.Get().Panel.PassHash == "" {
			a.p("Придумайте пароль для входа в панель (не меньше 6 символов).")
			if err := a.setPanelPassword(); err != nil {
				return err
			}
		}
		a.E.St.Update(func(st *store.State) { st.Panel.Port = port })
		bin, err := os.Executable()
		if err != nil {
			return err
		}
		if err := os.WriteFile(panelInit, []byte(panelScript(bin)), 0o755); err != nil {
			return fmt.Errorf("не удалось записать %s: %v", panelInit, err)
		}
		if err := panelCtl("restart"); err != nil {
			return err
		}
		time.Sleep(1500 * time.Millisecond)
		if !panelRunning() {
			out, _ := os.ReadFile("/opt/var/log/keenway-panel.log")
			return fmt.Errorf("панель не запустилась: %s", strings.TrimSpace(string(out)))
		}
		a.p("%s Веб-панель включена и запускается вместе с роутером.", a.green("✔"))
		a.panelURLs(port)
		return nil
	case "off", "remove":
		if _, err := os.Stat(panelInit); err != nil {
			a.p("Веб-панель не установлена.")
			return nil
		}
		panelCtl("stop")
		if err := os.Remove(panelInit); err != nil {
			return err
		}
		a.p("%s Веб-панель выключена и удалена из автозапуска.", a.green("✔"))
		return nil
	case "password":
		if err := a.setPanelPassword(); err != nil {
			return err
		}
		if panelRunning() {
			panelCtl("restart") // signs everyone out
		}
		a.p("%s Пароль панели изменён.", a.green("✔"))
		return nil
	case "restart":
		if _, err := os.Stat(panelInit); err != nil {
			return fmt.Errorf("веб-панель не установлена: keenway panel on")
		}
		return panelCtl("restart")
	case "status":
		if _, err := os.Stat(panelInit); err != nil {
			a.p("Веб-панель не установлена.")
			a.p("  Включить: %s — окно KeenWay в браузере с любого устройства дома, с паролем.", a.bold("keenway panel on"))
			return nil
		}
		if panelRunning() {
			a.p("%s Веб-панель работает.", a.green("●"))
		} else {
			a.p("%s Веб-панель установлена, но не запущена: keenway panel restart", a.red("●"))
		}
		port := a.E.St.Get().Panel.Port
		if port == 0 {
			port = DefaultPanelPort
		}
		a.panelURLs(port)
		a.p("  Сменить пароль: keenway panel password · выключить: keenway panel off")
		return nil
	}
	return fmt.Errorf("keenway panel on [порт] | off | password | status")
}

func (a *App) setPanelPassword() error {
	pw, err := a.askSecret("Пароль панели: ")
	if err != nil {
		return err
	}
	if len([]rune(pw)) < 6 {
		return fmt.Errorf("слишком короткий пароль — нужно не меньше 6 символов")
	}
	again, err := a.askSecret("Ещё раз: ")
	if err != nil {
		return err
	}
	if again != pw {
		return fmt.Errorf("пароли не совпали")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pw), 9)
	if err != nil {
		return err
	}
	return a.E.St.Update(func(st *store.State) { st.Panel.PassHash = string(hash) })
}

func (a *App) panelURLs(port int) {
	ips := lanIPs()
	if len(ips) == 0 {
		ips = []string{"192.168.1.1"}
	}
	for _, ip := range ips {
		a.p("  Адрес: %s", a.bold(fmt.Sprintf("http://%s:%d", ip, port)))
	}
	a.p("  %s", a.dim("Открывается только из домашней сети; из интернета панель недоступна."))
}

// lanIPs are the router's addresses in the home network (the br* bridges).
func lanIPs() []string {
	var out []string
	ifs, _ := net.Interfaces()
	for _, i := range ifs {
		if !strings.HasPrefix(i.Name, "br") || i.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, _ := i.Addrs()
		for _, ad := range addrs {
			if n, ok := ad.(*net.IPNet); ok && n.IP.To4() != nil && n.IP.IsPrivate() {
				out = append(out, n.IP.String())
			}
		}
	}
	return out
}

func panelCtl(action string) error {
	out, err := exec.Command("sh", panelInit, action).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %v %s", panelInit, action, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func panelRunning() bool {
	out, _ := exec.Command("sh", panelInit, "status").Output()
	return strings.TrimSpace(string(out)) == "alive"
}

func busy(port int) bool {
	c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
	if err != nil {
		return false
	}
	c.Close()
	return true
}
