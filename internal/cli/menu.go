package cli

import (
	"context"
	"strconv"
	"strings"

	"keenway/internal/core"
)

type item struct {
	label string
	run   func(ctx context.Context) error
}

// menu is the XKeen-style numbered menu shown when keenway runs without arguments.
func (a *App) menu(ctx context.Context) {
	items := []item{
		{"Статус и подписки", a.status},
		{"Добавить ключ или подписку", a.menuAdd},
		{"Подписки: интервал обновления, удалить", a.menuSubs},
		{"Сервисы (YouTube, Discord…): вкл/выкл, VPN/напрямую", a.menuServices},
		{"Свои сайты", a.menuSites},
		{"Устройства: правило для каждого", a.menuDevices},
		{"Серверы: пинг и выбор", a.menuServers},
		{"Трафик", a.traffic},
		{"Проверить и применить на роутере", a.menuApply},
		{"Настройки", a.menuSettings},
	}
	for {
		a.p("")
		pending := ""
		if core.Pending(a.E.St.Get()) {
			pending = a.yellow("  ● есть неприменённые изменения")
		}
		a.p("%s%s", a.bold("KeenWay "+a.Version), pending)
		for i, it := range items {
			a.p("  %2d. %s", i+1, it.label)
		}
		a.p("   0. Выход")
		choice := a.ask("> ")
		if choice == "0" || choice == "" || choice == "q" {
			return
		}
		n, err := strconv.Atoi(choice)
		if err != nil || n < 1 || n > len(items) {
			a.p("Нет такого пункта")
			continue
		}
		a.p("")
		if err := items[n-1].run(ctx); err != nil {
			a.p("%s %v", a.red("✖"), err)
		}
	}
}

func (a *App) menuAdd(context.Context) error {
	a.p("Вставьте ключ (vless://, hysteria2://, trojan://, ss://, vmess://), подписку https://… или happ://add/…")
	a.p("Для WireGuard/AmneziaWG вставьте конфиг целиком. Пустая строка — конец ввода.")
	var lines []string
	for {
		line := a.ask("")
		if line == "" {
			break
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return nil
	}
	return a.add(strings.Join(lines, "\n"))
}

func (a *App) menuSubs(ctx context.Context) error {
	if err := a.listSubs(); err != nil || len(a.E.St.Get().Settings.Subscriptions) == 0 {
		return err
	}
	name := a.ask("Название подписки (Enter — назад): ")
	if name == "" {
		return nil
	}
	switch a.ask("1 — интервал обновления, 2 — проверить (дни и трафик), 3 — удалить: ") {
	case "1":
		return a.subCmd(ctx, []string{"interval", name, a.ask("Как часто обновлять (например 30m, 6h, 1d): ")})
	case "2":
		return a.subCmd(ctx, []string{"check", name})
	case "3":
		if a.confirm("Удалить подписку " + name + "?") {
			return a.subCmd(ctx, []string{"del", name})
		}
	}
	return nil
}

func (a *App) menuServices(context.Context) error {
	if err := a.listServices(); err != nil {
		return err
	}
	id := a.ask("id сервиса (Enter — назад): ")
	if id == "" {
		return nil
	}
	action := map[string]string{"1": "on", "2": "off", "3": "vpn", "4": "direct"}[a.ask("1 — включить, 2 — выключить, 3 — через VPN, 4 — напрямую: ")]
	if action == "" {
		return nil
	}
	return a.serviceCmd([]string{id, action})
}

func (a *App) menuSites(context.Context) error {
	if err := a.listSites(); err != nil {
		return err
	}
	switch a.ask("1 — добавить сайт, 2 — удалить, Enter — назад: ") {
	case "1":
		name := a.ask("Название (будет видно в zashboard): ")
		if name == "" {
			return nil
		}
		targets := a.ask("Домены, IP или порты через пробел (например kinopoisk.ru hd.kinopoisk.ru): ")
		args := []string{"add", name}
		args = append(args, strings.Fields(targets)...)
		policy := map[string]string{"1": "--group", "2": "--vpn", "3": "--direct", "4": "--block"}[a.ask("Куда: 1 — своя плитка в zashboard, 2 — через VPN, 3 — напрямую, 4 — блокировать: ")]
		if policy != "" {
			args = append(args, policy)
		}
		if icon := a.ask("Ссылка на иконку (Enter — без иконки): "); icon != "" {
			args = append(args, "--icon", icon)
		}
		return a.siteCmd(args)
	case "2":
		name := a.ask("Название сайта: ")
		if name != "" && a.confirm("Удалить «"+name+"»?") {
			return a.siteCmd([]string{"del", name})
		}
	}
	return nil
}

func (a *App) menuDevices(ctx context.Context) error {
	if err := a.listDevices(ctx); err != nil {
		return err
	}
	dev := a.ask("IP или имя устройства (Enter — назад): ")
	if dev == "" {
		return nil
	}
	rule := map[string]string{"1": "direct", "2": "vpn", "3": "block", "4": "default", "5": "pin"}[a.ask("1 — всегда напрямую, 2 — всегда через VPN, 3 — без интернета, 4 — как у всех, 5 — закрепить IP: ")]
	if rule == "" {
		return nil
	}
	return a.deviceCmd(ctx, []string{dev, rule})
}

func (a *App) menuServers(ctx context.Context) error {
	if err := a.servers(ctx, a.confirm("Проверить скорость всех серверов?")); err != nil {
		return err
	}
	if choice := a.ask("Номер сервера для VPN (Enter — оставить): "); choice != "" {
		return a.use(ctx, []string{choice})
	}
	return nil
}

func (a *App) menuApply(ctx context.Context) error {
	if !core.Pending(a.E.St.Get()) {
		a.p("Изменений нет — на роутере уже актуальный конфиг.")
		return nil
	}
	if err := a.apply(ctx, true); err != nil {
		return err
	}
	if a.confirm("Применить на роутере?") {
		return a.apply(ctx, false)
	}
	return nil
}

func (a *App) menuSettings(ctx context.Context) error {
	st := a.E.St.Get()
	mode := map[string]string{"simple": "простой (VPN + самый быстрый)", "extended": "расширенный (Auto-Best + Fallback + ручной)"}[st.Settings.Mode]
	ads := map[bool]string{true: "вкл", false: "выкл"}[st.Settings.BlockAds]
	pass := map[bool]string{true: "установлен", false: "нет"}[st.Settings.Secret != ""]
	a.p("  1. Режим выбора сервера: %s", mode)
	a.p("  2. Блокировка рекламы: %s", ads)
	a.p("  3. Пароль панели zashboard: %s", pass)
	a.p("  4. Автообновление шаблона rockblack (update-config.sh)")
	a.p("  5. Откатить последнее применение")
	a.p("  6. Резервные копии")
	a.p("  7. Показать итоговый config.yaml")
	switch a.ask("> ") {
	case "1":
		next := "extended"
		if st.Settings.Mode == "extended" {
			next = "simple"
		}
		return a.mode([]string{next})
	case "2":
		return a.ads([]string{map[bool]string{true: "off", false: "on"}[st.Settings.BlockAds]})
	case "3":
		switch a.ask("1 — поставить/сменить, 2 — показать, 3 — убрать: ") {
		case "1":
			return a.password([]string{"set"})
		case "2":
			return a.password([]string{"show"})
		case "3":
			return a.password([]string{"off"})
		}
	case "4":
		if err := a.updater(ctx, nil); err != nil {
			return err
		}
		if a.confirm("Отключить его?") {
			return a.updater(ctx, []string{"off"})
		}
	case "5":
		if a.confirm("Вернуть конфиг, который был до последнего применения?") {
			return a.command(ctx, []string{"rollback"})
		}
	case "6":
		return a.command(ctx, []string{"backups"})
	case "7":
		return a.command(ctx, []string{"config"})
	}
	return nil
}
