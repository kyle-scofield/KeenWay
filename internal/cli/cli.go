// Package cli is the terminal front-end that runs on the router itself (Entware shell):
// an XKeen-style numbered menu plus one-line commands.
package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"keenway/internal/core"
	"keenway/internal/gen"
	"keenway/internal/links"
	"keenway/internal/mihomo"
	"keenway/internal/router"
	"keenway/internal/store"
	"keenway/internal/sub"
)

// App is one run of the terminal program.
type App struct {
	E       *core.Engine
	In      *bufio.Reader
	Out     io.Writer
	Version string
	color   bool
}

func New(st *store.Store, version string) *App {
	fi, _ := os.Stdout.Stat()
	return &App{
		E:       &core.Engine{St: st, MihomoHost: "127.0.0.1", Connect: func(context.Context) (*router.Conn, error) { return router.Local(), nil }},
		In:      bufio.NewReader(os.Stdin),
		Out:     os.Stdout,
		Version: version,
		color:   fi != nil && fi.Mode()&os.ModeCharDevice != 0 && os.Getenv("NO_COLOR") == "",
	}
}

// ---------- output helpers ----------

func (a *App) paint(code, s string) string {
	if !a.color {
		return s
	}
	return "\033[" + code + "m" + s + "\033[0m"
}
func (a *App) bold(s string) string   { return a.paint("1", s) }
func (a *App) green(s string) string  { return a.paint("32", s) }
func (a *App) red(s string) string    { return a.paint("31", s) }
func (a *App) yellow(s string) string { return a.paint("33", s) }
func (a *App) dim(s string) string    { return a.paint("2", s) }
func (a *App) p(format string, args ...any) {
	fmt.Fprintf(a.Out, format+"\n", args...)
}

func size(n int64) string {
	switch {
	case n >= 1e9:
		return fmt.Sprintf("%.1f ГБ", float64(n)/1e9)
	case n >= 1e6:
		return fmt.Sprintf("%.0f МБ", float64(n)/1e6)
	default:
		return fmt.Sprintf("%d КБ", n/1000)
	}
}

func plural(n int, one, few, many string) string {
	m10, m100 := n%10, n%100
	switch {
	case m10 == 1 && m100 != 11:
		return one
	case m10 >= 2 && m10 <= 4 && (m100 < 10 || m100 >= 20):
		return few
	}
	return many
}

func pad(s string, n int) string {
	if l := len([]rune(s)); l < n {
		return s + strings.Repeat(" ", n-l)
	}
	return s
}

func (a *App) ask(prompt string) string {
	fmt.Fprint(a.Out, prompt)
	line, _ := a.In.ReadString('\n')
	return strings.TrimSpace(line)
}

func (a *App) confirm(prompt string) bool {
	ans := strings.ToLower(a.ask(prompt + " [y/N]: "))
	return ans == "y" || ans == "д" || ans == "да" || ans == "yes"
}

// ParseInterval accepts "30m", "6h", "2d" (also "30м", "6ч", "2д") or plain seconds.
func ParseInterval(s string) (int, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	mult := 1
	for suffix, m := range map[string]int{"m": 60, "м": 60, "h": 3600, "ч": 3600, "d": 86400, "д": 86400} {
		if strings.HasSuffix(s, suffix) {
			s, mult = strings.TrimSuffix(s, suffix), m
			break
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("интервал вида 30m, 6h или 2d")
	}
	if n*mult < 60 {
		return 0, fmt.Errorf("интервал не меньше минуты")
	}
	return n * mult, nil
}

func humanInterval(sec int) string {
	switch {
	case sec%86400 == 0:
		return fmt.Sprintf("%d дн", sec/86400)
	case sec%3600 == 0:
		return fmt.Sprintf("%d ч", sec/3600)
	default:
		return fmt.Sprintf("%d мин", sec/60)
	}
}

// ---------- entry ----------

// useRouterTimezone makes dates match the router's clock: Entware has no zoneinfo,
// so Go would otherwise print everything in UTC.
func useRouterTimezone() {
	out, err := exec.Command("date", "+%z").Output()
	if err != nil {
		return
	}
	z := strings.TrimSpace(string(out))
	if len(z) != 5 {
		return
	}
	h, err1 := strconv.Atoi(z[1:3])
	m, err2 := strconv.Atoi(z[3:5])
	if err1 != nil || err2 != nil {
		return
	}
	off := h*3600 + m*60
	if z[0] == '-' {
		off = -off
	}
	time.Local = time.FixedZone(z, off)
}

// Run executes one command (or the menu when args is empty) and returns the exit code.
func (a *App) Run(args []string) int {
	useRouterTimezone()
	ctx := context.Background()
	if err := a.ensureImported(ctx); err != nil {
		a.p("%s %v", a.red("✖"), err)
		return 1
	}
	if len(args) == 0 {
		a.menu(ctx)
		return 0
	}
	if err := a.command(ctx, args); err != nil {
		a.p("%s %v", a.red("✖"), err)
		return 1
	}
	return 0
}

// ensureImported reads the router's current config on the very first run so nothing is lost.
func (a *App) ensureImported(ctx context.Context) error {
	if a.E.St.Get().Imported {
		return nil
	}
	c, _ := a.E.Connect(ctx)
	warnings, err := a.E.Import(ctx, c)
	if err != nil {
		return fmt.Errorf("первый запуск: %v", err)
	}
	a.p("%s Загружен текущий конфиг mihomo: подписки, сервисы и ваши правила сохранены.", a.green("✔"))
	for _, w := range warnings {
		a.p("  %s %s", a.yellow("!"), w)
	}
	return nil
}

func (a *App) command(ctx context.Context, args []string) error {
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "status", "s":
		return a.status(ctx)
	case "add":
		if len(rest) == 0 {
			return fmt.Errorf("укажите ключ или ссылку: keenway add <ключ|https://…>")
		}
		return a.add(strings.Join(rest, "\n"))
	case "subs":
		return a.listSubs()
	case "sub":
		return a.subCmd(ctx, rest)
	case "keys":
		return a.listKeys()
	case "key":
		return a.keyCmd(rest)
	case "services":
		return a.listServices()
	case "service":
		return a.serviceCmd(rest)
	case "sites":
		return a.listSites()
	case "site":
		return a.siteCmd(rest)
	case "devices":
		return a.listDevices(ctx)
	case "device":
		return a.deviceCmd(ctx, rest)
	case "servers":
		return a.servers(ctx, len(rest) > 0 && rest[0] == "test")
	case "use":
		return a.use(ctx, rest)
	case "traffic":
		return a.traffic(ctx)
	case "route", "check-site":
		return a.route(ctx, rest)
	case "profile":
		return a.profile(ctx, rest)
	case "mode":
		return a.mode(rest)
	case "ads":
		return a.ads(rest)
	case "apply":
		return a.apply(ctx, false)
	case "check":
		return a.apply(ctx, true)
	case "rollback":
		c, _ := a.E.Connect(ctx)
		if err := c.Rollback(ctx, a.E.St.Get().Settings.Secret); err != nil {
			return err
		}
		a.p("%s Возвращён прежний конфиг", a.green("✔"))
		return nil
	case "backups":
		c, _ := a.E.Connect(ctx)
		list, err := c.Backups(ctx)
		if err != nil {
			return err
		}
		for _, b := range list {
			a.p("  %s", b)
		}
		if len(list) == 0 {
			a.p("Резервных копий пока нет")
		}
		return nil
	case "config":
		cfg, err := core.Build(a.E.St.Get())
		if err != nil {
			return err
		}
		a.Out.Write(cfg)
		return nil
	case "import":
		if !a.confirm("Заново прочитать конфиг роутера? Несохранённые на роутере изменения KeenWay пропадут.") {
			return nil
		}
		a.E.St.Update(func(st *store.State) { st.Imported = false })
		return a.ensureImported(ctx)
	case "password":
		return a.password(rest)
	case "updater":
		return a.updater(ctx, rest)
	case "update":
		return a.selfUpdate()
	case "uninstall":
		return a.uninstall()
	case "version", "-v", "--version":
		a.p("KeenWay %s", a.Version)
		return nil
	case "help", "-h", "--help":
		a.help()
		return nil
	}
	return fmt.Errorf("неизвестная команда %q — см. keenway help", cmd)
}

func (a *App) help() {
	a.p("%s — настройка XKeen/mihomo без ручной правки конфигов", a.bold("KeenWay "+a.Version))
	a.p("")
	a.p("  keenway                         меню")
	a.p("  keenway status                  VPN, текущий сервер, подписки (дни и трафик)")
	a.p("  keenway add <ключ|ссылка>       добавить ключ (vless/hy2/trojan/ss/vmess/WireGuard) или подписку")
	a.p("  keenway subs | keys             подписки | отдельные ключи")
	a.p("  keenway sub del|interval|check <имя> [6h]")
	a.p("  keenway key del <номер>")
	a.p("  keenway services                сервисы (YouTube, Discord…)")
	a.p("  keenway service <id> on|off|vpn|direct")
	a.p("  keenway sites                   свои сайты")
	a.p("  keenway site add <название> <домен…> [--vpn|--direct|--block|--group] [--icon URL]")
	a.p("  keenway site del <название>")
	a.p("  keenway devices                 устройства в сети")
	a.p("  keenway device <IP|имя> direct|vpn|block|default   правило для устройства")
	a.p("  keenway device <IP|имя> pin     закрепить IP за устройством")
	a.p("  keenway servers [test]          серверы и пинг")
	a.p("  keenway use <сервер>            выбрать сервер для VPN")
	a.p("  keenway traffic                 трафик по устройствам и сервисам")
	a.p("  keenway route <сайт>            через что идёт сайт: правило и сервер")
	a.p("  keenway profile <режим>         normal | vpn | direct | games — все плитки сразу")
	a.p("  keenway mode simple|extended    режим выбора сервера")
	a.p("  keenway ads on|off              блокировка рекламы")
	a.p("  keenway check | apply           проверить | применить (с автооткатом)")
	a.p("  keenway rollback | backups      откат | резервные копии")
	a.p("  keenway password set|show|off   пароль панели zashboard (порт 9090)")
	a.p("  keenway updater status|off      автообновление шаблона rockblack (update-config.sh)")
	a.p("  keenway config                  показать итоговый config.yaml")
	a.p("  keenway update | uninstall      обновить | удалить KeenWay")
}

// ---------- status ----------

func (a *App) status(ctx context.Context) error {
	m := a.E.Mihomo(ctx)
	v, err := m.Version(ctx)
	if err != nil {
		a.p("%s mihomo не отвечает: %v", a.red("●"), err)
		a.p("  Проверьте: xkeen -status")
	} else {
		a.p("%s mihomo %s работает", a.green("●"), v)
	}
	if groups, err := m.Groups(ctx); err == nil {
		byName := map[string]mihomo.Group{}
		for _, g := range groups {
			byName[g.Name] = g
		}
		if vpn, ok := byName["VPN"]; ok {
			now := vpn.Now
			if inner, ok := byName[now]; ok && inner.Now != "" {
				now += " → " + inner.Now
			}
			a.p("  VPN идёт через: %s", a.bold(now))
		}
	}
	if providers, err := m.Providers(ctx); err == nil {
		a.p("")
		a.p("%s", a.bold("Подписки"))
		for _, p := range providers {
			a.p("  %s  %s", a.bold(p.Name), a.dim(fmt.Sprintf("живых серверов %d из %d", p.Alive, len(p.Nodes))))
			a.p("    %s", a.subscriptionLine(p.Info))
		}
		if len(providers) == 0 {
			a.p("  нет")
		}
	}
	if core.Pending(a.E.St.Get()) {
		a.p("")
		a.p("%s Есть неприменённые изменения — keenway apply", a.yellow("!"))
	}
	return nil
}

// subscriptionLine formats quota and expiry like Happ does.
func (a *App) subscriptionLine(info *mihomo.SubscriptionInfo) string {
	if info == nil {
		return a.dim("провайдер не сообщает срок и трафик")
	}
	used := info.Upload + info.Download
	traffic := size(used) + " · безлимит"
	if info.Total > 0 {
		pct := float64(used) / float64(info.Total) * 100
		bar := strings.Repeat("█", int(pct/10)) + strings.Repeat("░", 10-min(10, int(pct/10)))
		traffic = fmt.Sprintf("%s %s из %s", bar, size(used), size(info.Total))
	}
	if info.Expire == 0 {
		return traffic + " · без срока"
	}
	left := int(time.Until(time.Unix(info.Expire, 0)).Hours() / 24)
	if left < 0 {
		left = 0
	}
	daysText := fmt.Sprintf("осталось %d %s (до %s)", left, plural(left, "день", "дня", "дней"), time.Unix(info.Expire, 0).Format("02.01.2006"))
	switch {
	case left <= 3:
		daysText = a.red(daysText)
	case left <= 7:
		daysText = a.yellow(daysText)
	default:
		daysText = a.green(daysText)
	}
	return traffic + " · " + daysText
}

// ---------- keys & subscriptions ----------

func (a *App) add(text string) error {
	items := links.Parse(text)
	st := a.E.St.Get()
	var keys []string
	var subs []gen.Subscription
	for _, it := range items {
		switch {
		case it.Err != nil:
			a.p("  %s %s — %v", a.red("✖"), shorten(it.Raw, 50), it.Err)
		case it.Kind == "subscription":
			name := subscriptionName(it.URL, append(st.Settings.Subscriptions, subs...))
			subs = append(subs, gen.Subscription{Name: name, URL: it.URL, Interval: 3600, Headers: core.DefaultHeaders(st)})
			a.p("  %s подписка %s", a.green("✔"), a.bold(name))
		default:
			keys = append(keys, it.Raw)
			a.p("  %s %s %s %s", a.green("✔"), it.Proxy.Values["type"], a.bold(it.Proxy.Name()), a.dim(fmt.Sprint(it.Proxy.Values["server"])))
		}
	}
	if len(keys)+len(subs) == 0 {
		return fmt.Errorf("ничего не добавлено")
	}
	if err := a.E.St.Update(func(st *store.State) {
		st.Keys = append(st.Keys, keys...)
		st.Settings.Subscriptions = append(st.Settings.Subscriptions, subs...)
	}); err != nil {
		return err
	}
	a.p("Сохранено. Применить на роутере: %s", a.bold("keenway apply"))
	return nil
}

func shorten(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

func subscriptionName(url string, existing []gen.Subscription) string {
	base := "sub"
	if i := strings.Index(url, "://"); i >= 0 {
		host := url[i+3:]
		if j := strings.IndexAny(host, "/:?"); j >= 0 {
			host = host[:j]
		}
		parts := strings.Split(host, ".")
		if len(parts) >= 2 {
			base = parts[len(parts)-2]
		}
	}
	name, n := base, 2
	for {
		taken := false
		for _, s := range existing {
			if s.Name == name {
				taken = true
			}
		}
		if !taken {
			return name
		}
		name = fmt.Sprintf("%s-%d", base, n)
		n++
	}
}

func (a *App) listSubs() error {
	st := a.E.St.Get()
	if len(st.Settings.Subscriptions) == 0 {
		a.p("Подписок нет. Добавить: keenway add https://…")
		return nil
	}
	for _, s := range st.Settings.Subscriptions {
		a.p("  %s  обновлять каждые %s  %s", a.bold(pad(s.Name, 16)), humanInterval(orDefault(s.Interval, 3600)), a.dim(maskURL(s.URL)))
	}
	return nil
}

func orDefault(v, d int) int {
	if v <= 0 {
		return d
	}
	return v
}

func maskURL(u string) string {
	if i := strings.Index(u, "://"); i >= 0 {
		if j := strings.Index(u[i+3:], "/"); j >= 0 {
			return u[:i+3+j+1] + "••••••"
		}
	}
	return u
}

func (a *App) subCmd(ctx context.Context, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("keenway sub del|interval|check <имя> [6h]")
	}
	action, name := args[0], args[1]
	st := a.E.St.Get()
	idx := -1
	for i, s := range st.Settings.Subscriptions {
		if strings.EqualFold(s.Name, name) {
			idx = i
		}
	}
	if idx < 0 {
		return fmt.Errorf("подписка %q не найдена (keenway subs)", name)
	}
	switch action {
	case "del", "rm":
		return a.E.St.Update(func(st *store.State) {
			st.Settings.Subscriptions = append(st.Settings.Subscriptions[:idx], st.Settings.Subscriptions[idx+1:]...)
		})
	case "interval":
		if len(args) < 3 {
			return fmt.Errorf("укажите интервал: keenway sub interval %s 6h", name)
		}
		sec, err := ParseInterval(args[2])
		if err != nil {
			return err
		}
		return a.E.St.Update(func(st *store.State) { st.Settings.Subscriptions[idx].Interval = sec })
	case "check":
		s := st.Settings.Subscriptions[idx]
		res, err := sub.Fetch(ctx, s.URL, s.Headers)
		if err != nil {
			return err
		}
		a.p("  %s: %d серверов", a.bold(s.Name), len(res.Nodes))
		if res.HasInfo {
			info := &mihomo.SubscriptionInfo{Upload: res.Info.Upload, Download: res.Info.Download, Total: res.Info.Total}
			if !res.Info.Expire.IsZero() {
				info.Expire = res.Info.Expire.Unix()
			}
			a.p("  %s", a.subscriptionLine(info))
		}
		return nil
	}
	return fmt.Errorf("неизвестное действие %q", action)
}

func (a *App) listKeys() error {
	st := a.E.St.Get()
	n := 0
	for _, k := range st.Keys {
		for _, it := range links.Parse(k) {
			if it.Proxy != nil {
				n++
				a.p("  %d. %s %s %s", n, pad(fmt.Sprint(it.Proxy.Values["type"]), 10), a.bold(it.Proxy.Name()), a.dim(fmt.Sprint(it.Proxy.Values["server"])))
			}
		}
	}
	for _, raw := range st.RawProxies {
		if p, err := gen.ProxyFromYAML(raw); err == nil {
			a.p("  -  %s %s %s", pad(fmt.Sprint(p.Values["type"]), 10), a.bold(p.Name()), a.dim("(из конфига роутера)"))
		}
	}
	if n == 0 && len(st.RawProxies) == 0 {
		a.p("Отдельных ключей нет. Добавить: keenway add vless://…")
	}
	return nil
}

func (a *App) keyCmd(args []string) error {
	if len(args) < 2 || args[0] != "del" {
		return fmt.Errorf("keenway key del <номер из keenway keys>")
	}
	n, err := strconv.Atoi(args[1])
	st := a.E.St.Get()
	if err != nil || n < 1 || n > len(st.Keys) {
		return fmt.Errorf("нет ключа с номером %s", args[1])
	}
	return a.E.St.Update(func(st *store.State) { st.Keys = append(st.Keys[:n-1], st.Keys[n:]...) })
}

// ---------- services ----------

func (a *App) listServices() error {
	st := a.E.St.Get()
	on := map[string]bool{}
	for _, id := range st.Settings.Services {
		on[id] = true
	}
	for _, svc := range gen.Services {
		mark, route := a.dim("○ выкл"), ""
		if on[svc.ID] {
			mark = a.green("● вкл ")
			via := svc.ViaVPN
			if v, ok := st.Settings.ServiceVPN[svc.ID]; ok {
				via = v
			}
			route = "напрямую"
			if via {
				route = "через VPN"
			}
		}
		a.p("  %s %s %s %s", mark, pad(svc.ID, 12), pad(svc.Name, 20), a.dim(route))
	}
	return nil
}

func (a *App) serviceCmd(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("keenway service <id> on|off|vpn|direct")
	}
	svc, ok := gen.ServiceByID(args[0])
	if !ok {
		return fmt.Errorf("нет сервиса %q (keenway services)", args[0])
	}
	return a.E.St.Update(func(st *store.State) {
		set := map[string]bool{}
		for _, id := range st.Settings.Services {
			set[id] = true
		}
		if st.Settings.ServiceVPN == nil {
			st.Settings.ServiceVPN = map[string]bool{}
		}
		switch args[1] {
		case "on":
			set[svc.ID] = true
		case "off":
			delete(set, svc.ID)
		case "vpn", "direct":
			set[svc.ID] = true
			if via := args[1] == "vpn"; via == svc.ViaVPN {
				delete(st.Settings.ServiceVPN, svc.ID)
			} else {
				st.Settings.ServiceVPN[svc.ID] = via
			}
		}
		st.Settings.Services = nil
		for _, s := range gen.Services {
			if set[s.ID] {
				st.Settings.Services = append(st.Settings.Services, s.ID)
			}
		}
	})
}

// ---------- own sites ----------

func (a *App) listSites() error {
	st := a.E.St.Get()
	if len(st.Settings.Sites) == 0 {
		a.p("Своих сайтов нет. Добавить: keenway site add Кинопоиск kinopoisk.ru --group --icon https://…")
		return nil
	}
	for _, s := range st.Settings.Sites {
		where := map[gen.Policy]string{gen.PolicyVPN: "через VPN", gen.PolicyDirect: "напрямую", gen.PolicyBlock: "блокировать", gen.PolicyGroup: "своя плитка в zashboard"}[s.Policy]
		if s.Target != "" {
			if svc, ok := gen.ServiceByID(s.Target); ok {
				where = "как " + svc.Name
			}
		}
		a.p("  %s  %s", a.bold(s.Name), a.dim(where))
		var parts []string
		parts = append(parts, s.Domains...)
		parts = append(parts, s.IPs...)
		for _, p := range s.Ports {
			parts = append(parts, "порт "+p)
		}
		a.p("    %s", strings.Join(parts, ", "))
	}
	return nil
}

func (a *App) siteCmd(args []string) error {
	if len(args) >= 2 && (args[0] == "del" || args[0] == "rm") {
		name := strings.Join(args[1:], " ")
		found := false
		err := a.E.St.Update(func(st *store.State) {
			for i, s := range st.Settings.Sites {
				if strings.EqualFold(s.Name, name) {
					st.Settings.Sites = append(st.Settings.Sites[:i], st.Settings.Sites[i+1:]...)
					found = true
					return
				}
			}
		})
		if err == nil && !found {
			return fmt.Errorf("сайт %q не найден", name)
		}
		return err
	}
	if len(args) < 3 || args[0] != "add" {
		return fmt.Errorf("keenway site add <название> <домен|IP|порт…> [--vpn|--direct|--block|--group|--as <сервис>] [--icon URL]")
	}
	site := gen.CustomSite{Name: args[1], Policy: gen.PolicyGroup}
	for i := 2; i < len(args); i++ {
		v := args[i]
		switch {
		case v == "--vpn":
			site.Policy = gen.PolicyVPN
		case v == "--direct":
			site.Policy = gen.PolicyDirect
		case v == "--block":
			site.Policy = gen.PolicyBlock
		case v == "--group":
			site.Policy = gen.PolicyGroup
		case v == "--icon" && i+1 < len(args):
			i++
			site.Icon = args[i]
		case v == "--as" && i+1 < len(args):
			i++
			if _, ok := gen.ServiceByID(args[i]); !ok {
				return fmt.Errorf("нет сервиса %q", args[i])
			}
			site.Target, site.Policy = args[i], gen.PolicyVPN
		case looksLikePort(v):
			site.Ports = append(site.Ports, v)
		case looksLikeIP(v):
			site.IPs = append(site.IPs, v)
		default:
			site.Domains = append(site.Domains, v)
		}
	}
	probe := gen.Settings{Subscriptions: []gen.Subscription{{Name: "x", URL: "https://x"}}, Sites: []gen.CustomSite{site}}
	if err := probe.Validate(); err != nil {
		return err
	}
	if err := a.E.St.Update(func(st *store.State) { st.Settings.Sites = append(st.Settings.Sites, site) }); err != nil {
		return err
	}
	a.p("%s Сайт «%s» добавлен. Применить: keenway apply", a.green("✔"), site.Name)
	return nil
}

func looksLikePort(s string) bool {
	for _, r := range s {
		if (r < '0' || r > '9') && r != '-' {
			return false
		}
	}
	return s != "" && !strings.HasPrefix(s, "-")
}

func looksLikeIP(s string) bool {
	return strings.Count(s, ".") == 3 && strings.Trim(s, "0123456789./") == "" || strings.Contains(s, ":") && strings.Contains(s, "/")
}

// ---------- devices ----------

func (a *App) listDevices(ctx context.Context) error {
	c, _ := a.E.Connect(ctx)
	devs, err := c.Devices(ctx)
	if err != nil {
		return err
	}
	rules := map[string]gen.Policy{}
	for _, d := range a.E.St.Get().Settings.Devices {
		rules[d.IP] = d.Policy
	}
	sort.Slice(devs, func(i, j int) bool { return devs[i].Active && !devs[j].Active })
	for _, d := range devs {
		if d.IP == "" {
			continue
		}
		dot := a.dim("○")
		if d.Active {
			dot = a.green("●")
		}
		rule := map[gen.Policy]string{gen.PolicyDirect: a.yellow("всегда напрямую"), gen.PolicyVPN: a.green("всегда через VPN"), gen.PolicyBlock: a.red("без интернета")}[rules[d.IP]]
		a.p("  %s %s %s %s %s", dot, pad(d.Label(), 22), pad(d.IP, 15), a.dim(pad(size(d.RX+d.TX), 8)), rule)
	}
	return nil
}

func (a *App) deviceCmd(ctx context.Context, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("keenway device <IP|имя> direct|vpn|block|default|pin")
	}
	c, _ := a.E.Connect(ctx)
	devs, _ := c.Devices(ctx)
	var dev *router.Device
	for i, d := range devs {
		if d.IP == args[0] || strings.EqualFold(d.Label(), args[0]) || strings.EqualFold(d.MAC, args[0]) {
			dev = &devs[i]
		}
	}
	name, ip := args[0], args[0]
	if dev != nil {
		name, ip = dev.Label(), dev.IP
	}
	if !looksLikeIP(ip) {
		return fmt.Errorf("устройство %q не найдено — укажите IP", args[0])
	}
	if args[1] == "pin" {
		if dev == nil {
			return fmt.Errorf("закрепить IP можно только для устройства из списка роутера")
		}
		if err := c.PinIP(ctx, dev.MAC, dev.IP); err != nil {
			return err
		}
		a.p("%s За «%s» закреплён %s", a.green("✔"), name, ip)
		return nil
	}
	policy := gen.Policy(args[1])
	if args[1] != "default" && policy != gen.PolicyDirect && policy != gen.PolicyVPN && policy != gen.PolicyBlock {
		return fmt.Errorf("правило: direct, vpn, block или default")
	}
	return a.E.St.Update(func(st *store.State) {
		var keep []gen.DeviceRule
		for _, d := range st.Settings.Devices {
			if d.IP != ip {
				keep = append(keep, d)
			}
		}
		if args[1] != "default" {
			keep = append(keep, gen.DeviceRule{Name: name, IP: ip, Policy: policy})
		}
		st.Settings.Devices = keep
	})
}

// ---------- servers & traffic ----------

func delayText(a *App, d int, known bool) string {
	switch {
	case !known:
		return a.dim("   —")
	case d == 0:
		return a.red("  нет")
	case d < 300:
		return a.green(fmt.Sprintf("%4d мс", d))
	case d < 800:
		return a.yellow(fmt.Sprintf("%4d мс", d))
	}
	return a.red(fmt.Sprintf("%4d мс", d))
}

func (a *App) servers(ctx context.Context, test bool) error {
	m := a.E.Mihomo(ctx)
	delays := map[string]int{}
	known := map[string]bool{}
	if providers, err := m.Providers(ctx); err == nil {
		for _, p := range providers {
			for _, n := range p.Nodes {
				delays[n.Name], known[n.Name] = n.Delay, n.Delay > 0
			}
		}
	}
	if test {
		a.p("Проверка серверов…")
		res, err := m.TestGroup(ctx, "VPN")
		if err != nil {
			return err
		}
		for k, v := range res {
			delays[k], known[k] = v, true
		}
	}
	groups, err := m.Groups(ctx)
	if err != nil {
		return err
	}
	for _, g := range groups {
		if g.Name != "VPN" {
			continue
		}
		for i, o := range g.Options {
			mark := "  "
			if o == g.Now {
				mark = a.green("✔ ")
			}
			a.p("  %s%2d. %s %s", mark, i+1, pad(o, 40), delayText(a, delays[o], known[o]))
		}
	}
	a.p("Выбрать: %s", a.bold("keenway use <номер или название>"))
	return nil
}

func (a *App) use(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("keenway use <номер или название сервера>")
	}
	m := a.E.Mihomo(ctx)
	groups, err := m.Groups(ctx)
	if err != nil {
		return err
	}
	want := strings.Join(args, " ")
	for _, g := range groups {
		if g.Name != "VPN" {
			continue
		}
		choice := ""
		if n, err := strconv.Atoi(want); err == nil && n >= 1 && n <= len(g.Options) {
			choice = g.Options[n-1]
		}
		for _, o := range g.Options {
			if choice == "" && strings.Contains(strings.ToLower(o), strings.ToLower(want)) {
				choice = o
			}
		}
		if choice == "" {
			return fmt.Errorf("сервер %q не найден (keenway servers)", want)
		}
		if err := m.Select(ctx, "VPN", choice); err != nil {
			return err
		}
		a.p("%s VPN → %s", a.green("✔"), choice)
		return nil
	}
	return fmt.Errorf("в конфиге нет группы VPN")
}

func (a *App) traffic(ctx context.Context) error {
	m := a.E.Mihomo(ctx)
	snap, err := m.Traffic(ctx)
	if err != nil {
		return err
	}
	a.p("%s ↑ %s ↓ %s, открытых соединений %d", a.bold("С запуска mihomo:"), size(snap.UploadTotal), size(snap.DownloadTotal), len(snap.Connections))
	c, _ := a.E.Connect(ctx)
	if devs, err := c.Devices(ctx); err == nil {
		a.p("")
		a.p("%s", a.bold("Устройства (всего за время подключения, по данным роутера)"))
		sort.Slice(devs, func(i, j int) bool { return devs[i].RX+devs[i].TX > devs[j].RX+devs[j].TX })
		for _, d := range devs[:min(10, len(devs))] {
			if d.RX+d.TX > 0 {
				a.p("  %s %s ↓ %s ↑ %s", pad(d.Label(), 22), pad(d.IP, 15), pad(size(d.RX), 9), size(d.TX))
			}
		}
	}
	a.p("")
	a.p("%s", a.bold("Сервисы (открытые соединения)"))
	for _, u := range snap.ByService()[:min(10, len(snap.ByService()))] {
		a.p("  %s %3d соед.  %s", pad(u.Key, 24), u.Connections, size(u.Upload+u.Download))
	}
	return nil
}

// ---------- settings ----------

func (a *App) mode(args []string) error {
	if len(args) == 0 || (args[0] != "simple" && args[0] != "extended") {
		return fmt.Errorf("keenway mode simple|extended")
	}
	return a.E.St.Update(func(st *store.State) { st.Settings.Mode = args[0] })
}

func (a *App) ads(args []string) error {
	if len(args) == 0 || (args[0] != "on" && args[0] != "off") {
		return fmt.Errorf("keenway ads on|off")
	}
	return a.E.St.Update(func(st *store.State) { st.Settings.BlockAds = args[0] == "on" })
}

func (a *App) password(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("keenway password set [пароль] | show | off")
	}
	switch args[0] {
	case "show":
		if s := a.E.St.Get().Settings.Secret; s != "" {
			a.p("Пароль панели: %s", a.bold(s))
		} else {
			a.p("Пароль панели не установлен")
		}
		return nil
	case "off":
		return a.E.St.Update(func(st *store.State) { st.Settings.Secret = "" })
	case "set":
		secret := ""
		if len(args) > 1 {
			secret = args[1]
		} else {
			secret = a.ask("Новый пароль панели: ")
		}
		if len(secret) < 6 {
			return fmt.Errorf("пароль не короче 6 символов")
		}
		if err := a.E.St.Update(func(st *store.State) { st.Settings.Secret = secret }); err != nil {
			return err
		}
		a.p("Сохранено. После keenway apply zashboard попросит этот пароль.")
		return nil
	}
	return fmt.Errorf("keenway password set|show|off")
}

func (a *App) updater(ctx context.Context, args []string) error {
	c, _ := a.E.Connect(ctx)
	on, err := c.TemplateUpdaterEnabled(ctx)
	if err != nil {
		return err
	}
	if len(args) > 0 && args[0] == "off" {
		if !on {
			a.p("Уже выключено")
			return nil
		}
		if err := c.DisableTemplateUpdater(ctx); err != nil {
			return err
		}
		a.p("%s update-config.sh отключён в планировщике (копия: crontabs/root.keenway-bak)", a.green("✔"))
		return nil
	}
	if on {
		a.p("%s Включено: update-config.sh раз в несколько дней перезаписывает группы и правила и сотрёт настройки KeenWay.", a.yellow("!"))
		a.p("  Отключить: keenway updater off")
	} else {
		a.p("%s Выключено", a.green("✔"))
	}
	return nil
}

// ---------- install management ----------

// Repo is the GitHub "owner/name" releases come from; set at build time.
var Repo = "scroge071-cloud/KeenWay"

func (a *App) selfUpdate() error {
	url := "https://raw.githubusercontent.com/" + Repo + "/main/install.sh"
	a.p("Обновление KeenWay с %s", url)
	cmd := exec.Command("sh", "-c", "curl -fsSL "+url+" | sh")
	cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
	return cmd.Run()
}

func (a *App) uninstall() error {
	if !a.confirm("Удалить KeenWay? Конфиг mihomo на роутере останется как есть.") {
		return nil
	}
	self, _ := os.Executable()
	if a.confirm("Удалить и сохранённые настройки KeenWay (/opt/etc/keenway)?") {
		os.RemoveAll(StateDir)
	}
	if err := os.Remove(self); err != nil {
		return err
	}
	a.p("%s KeenWay удалён", a.green("✔"))
	return nil
}

// StateDir is where the router build keeps its settings.
const StateDir = "/opt/etc/keenway"

// ---------- apply ----------

func (a *App) apply(ctx context.Context, dry bool) error {
	if !dry && !core.Pending(a.E.St.Get()) {
		a.p("Изменений нет — на роутере уже актуальный конфиг.")
		return nil
	}
	if dry {
		a.p("Проверка конфига…")
	} else {
		a.p("Применение настроек на роутере (обычно до минуты). Если пропадёт интернет — роутер сам вернётся к прежним настройкам.")
	}
	err := a.E.Apply(ctx, dry, func(s router.Step) {
		mark := a.green("✔")
		if !s.OK {
			mark = a.red("✖")
		}
		if s.Info != "" {
			a.p("  %s %s %s", mark, s.Name, a.dim(s.Info))
		} else {
			a.p("  %s %s", mark, s.Name)
		}
	})
	if err != nil {
		return err
	}
	if dry {
		a.p("%s Конфиг корректен, можно применять: keenway apply", a.green("✔"))
	} else {
		a.p("%s Применено. Интернет и VPN работают.", a.green("✔"))
	}
	return nil
}

func (a *App) route(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("укажите сайт: keenway route youtube.com")
	}
	host, port, err := core.ParseTarget(args[0])
	if err != nil {
		return err
	}
	r, err := a.E.Mihomo(ctx).Route(ctx, host, port)
	if err != nil {
		return err
	}
	last := r.Chain[len(r.Chain)-1]
	switch last {
	case "DIRECT":
		a.p("%s %s идёт %s", a.yellow("➜"), host, a.bold("напрямую, без VPN"))
	case "REJECT", "REJECT-DROP":
		a.p("%s %s %s", a.red("✖"), host, a.bold("заблокирован"))
	default:
		a.p("%s %s идёт %s", a.green("✔"), host, a.bold("через VPN"))
	}
	a.p("  путь:    %s", strings.Join(r.Chain, " → "))
	a.p("  правило: %s %s", r.Rule, r.Payload)
	return nil
}

func (a *App) profile(ctx context.Context, args []string) error {
	if len(args) == 0 {
		for _, p := range core.Profiles {
			a.p("  %-8s %s — %s", p.ID, a.bold(p.Name), a.dim(p.About))
		}
		return nil
	}
	n, err := core.ApplyProfile(ctx, a.E.Mihomo(ctx), args[0])
	if err != nil {
		return err
	}
	a.p("%s Режим переключён, изменено плиток: %d", a.green("✔"), n)
	return nil
}
