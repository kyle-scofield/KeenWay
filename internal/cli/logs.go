package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"keenway/internal/mihomo"
)

// logs shows mihomo's live log or the router's journals in the terminal.
//
//	keenway logs [error|warning|info|debug] [слово]   живой лог (Ctrl+C — выход)
//	keenway logs router [слово]                       журналы роутера
func (a *App) logs(ctx context.Context, args []string) error {
	if len(args) > 0 && (args[0] == "router" || args[0] == "files") {
		return a.routerLogs(ctx, strings.Join(args[1:], " "))
	}
	level := "info"
	if len(args) > 0 {
		switch args[0] {
		case "debug", "info", "warning", "error":
			level, args = args[0], args[1:]
		}
	}
	filter := strings.ToLower(strings.TrimSpace(strings.Join(args, " ")))
	// Ctrl+C ends the log and returns to the menu instead of closing the program
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()
	a.p("%s Живой лог mihomo (уровень %s%s). Выход: Ctrl+C", a.green("●"), level, map[bool]string{true: ", фильтр «" + filter + "»", false: ""}[filter != ""])
	return a.E.Mihomo(ctx).Logs(ctx, level, func(l mihomo.LogLine) {
		line := l.Message
		if filter != "" && !strings.Contains(strings.ToLower(line), filter) {
			return
		}
		lv := fmt.Sprintf("%-7s", l.Level)
		switch l.Level {
		case "error":
			lv, line = a.red(lv), a.red(line)
		case "warning":
			lv = a.yellow(lv)
		default:
			lv = a.dim(lv)
		}
		a.p("%s %s %s", a.dim(l.Time.Format("15:04:05")), lv, line)
	})
}

// routerLogs prints the router's journals (last 60 lines each, or the lines with the word).
func (a *App) routerLogs(ctx context.Context, filter string) error {
	c, err := a.E.Connect(ctx)
	if err != nil {
		return err
	}
	defer c.Close()
	files := [][2]string{
		{"/tmp/keenway-test.log", "Последняя проверка конфига"},
		{"/tmp/keenway-xkeen.log", "Последний перезапуск XKeen"},
		{"/opt/var/log/xkeen-detached.log", "XKeen"},
		{"/opt/var/log/mihomo-update.log", "Обновление mihomo"},
		{"/opt/var/log/keenway-panel.log", "Веб-панель KeenWay"},
		{"/opt/var/log/keenway-update.log", "Обновление KeenWay"},
	}
	var sb strings.Builder
	for i, f := range files {
		fmt.Fprintf(&sb, "echo '@@KW %d'; [ -f %s ] && tail -n 400 %s 2>/dev/null; ", i, f[0], f[0])
	}
	sb.WriteString(`echo '@@KW mem'; free; echo '@@KW kernel'; dmesg 2>/dev/null | grep -iE 'oom|killed process|out of memory' | tail -n 20`)
	out, err := c.Shell(ctx, sb.String())
	if err != nil && out == "" {
		return err
	}
	filter = strings.ToLower(filter)
	var title string
	var lines []string
	flush := func() {
		if title == "" {
			return
		}
		if filter != "" {
			var keep []string
			for _, l := range lines {
				if strings.Contains(strings.ToLower(l), filter) {
					keep = append(keep, l)
				}
			}
			lines = keep
		} else if len(lines) > 60 {
			lines = lines[len(lines)-60:]
		}
		if len(lines) == 0 {
			return
		}
		a.p("")
		a.p("%s", a.bold("── "+title+" ──"))
		for _, l := range lines {
			low := strings.ToLower(l)
			if strings.Contains(low, "error") || strings.Contains(low, "fatal") || strings.Contains(low, "killed") {
				l = a.red(l)
			} else if strings.Contains(low, "warn") {
				l = a.yellow(l)
			}
			a.p("  %s", l)
		}
	}
	for _, line := range strings.Split(out, "\n") {
		if rest, ok := strings.CutPrefix(line, "@@KW "); ok {
			flush()
			title, lines = "", nil
			switch rest {
			case "mem":
				title = "Память"
			case "kernel":
				title = "Ядро: нехватка памяти"
			default:
				var i int
				fmt.Sscan(rest, &i)
				if i >= 0 && i < len(files) {
					title = files[i][1] + " (" + files[i][0] + ")"
				}
			}
			continue
		}
		if line != "" {
			lines = append(lines, line)
		}
	}
	flush()
	return nil
}
