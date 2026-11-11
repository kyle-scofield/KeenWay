package cli

import (
	"context"
	"fmt"
	"keenway/internal/update"
	"os"
	"strconv"
	"strings"
	"time"
)

// Letters in the "ANSI Shadow" style, six rows each.
var bigLetters = map[rune][6]string{
	'K': {"██╗  ██╗", "██║ ██╔╝", "█████╔╝ ", "██╔═██╗ ", "██║  ██╗", "╚═╝  ╚═╝"},
	'E': {"███████╗", "██╔════╝", "█████╗  ", "██╔══╝  ", "███████╗", "╚══════╝"},
	'N': {"███╗   ██╗", "████╗  ██║", "██╔██╗ ██║", "██║╚██╗██║", "██║ ╚████║", "╚═╝  ╚═══╝"},
	'W': {"██╗    ██╗", "██║    ██║", "██║ █╗ ██║", "██║███╗██║", "╚███╔███╔╝", " ╚══╝╚══╝ "},
	'A': {" █████╗ ", "██╔══██╗", "███████║", "██╔══██║", "██║  ██║", "╚═╝  ╚═╝"},
	'Y': {"██╗   ██╗", "╚██╗ ██╔╝", " ╚████╔╝ ", "  ╚██╔╝  ", "   ██║   ", "   ╚═╝   "},
}

// rgb paints with a 24-bit colour (falls back to plain text without colours).
func (a *App) rgb(r, g, b int, s string) string {
	return a.paint(fmt.Sprintf("1;38;2;%d;%d;%d", r, g, b), s)
}

// bigWord renders text with bigLetters.
func bigWord(text string) (rows [6]string) {
	for _, ch := range text {
		l := bigLetters[ch]
		for i := range rows {
			rows[i] += l[i]
		}
	}
	return rows
}

func termWidth() int {
	if n, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && n > 0 {
		return n
	}
	return 80
}

// banner prints the logo and one status line under it.
func (a *App) banner(ctx context.Context) {
	a.logo()
	if line := a.statusLine(ctx); line != "" {
		a.p("  %s", line)
	}
	if tag := a.newerVersion(ctx); tag != "" {
		a.p("  %s Вышла новая версия %s — обновить: %s", a.yellow("⬆"), a.bold(tag), a.bold("keenway update"))
	}
}

// logo prints the KeenWay logo — "Keen" in white, "Way" in the app's violet,
// shading downwards — and the tagline.
func (a *App) logo() {
	if a.logoDone {
		return
	}
	a.logoDone = true
	a.p("")
	if termWidth() < 64 {
		a.p("  %s%s  %s", a.rgb(236, 238, 247, "Keen"), a.rgb(139, 133, 255, "Way"), a.dim(a.Version))
	} else {
		keen, way := bigWord("KEEN"), bigWord("WAY")
		for i := 0; i < 6; i++ {
			// a soft vertical gradient: brighter at the top
			w := 255 - i*12
			v := [3]int{165 - i*8, 155 - i*10, 255 - i*4}
			a.p("  %s%s", a.rgb(w, w, w, keen[i]), a.rgb(v[0], v[1], v[2], way[i]))
		}
	}
	a.p("  %s", a.dim("XKeen / mihomo без ручной правки конфигов · "+a.Version))
}

// statusLine is a quick one-line summary: is VPN up, which server, subscription days.
func (a *App) statusLine(ctx context.Context) string {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	m := a.E.Mihomo(ctx)
	if _, err := m.Version(ctx); err != nil {
		return a.red("●") + " mihomo не отвечает"
	}
	parts := []string{a.green("●") + " VPN работает"}
	if groups, err := m.Groups(ctx); err == nil {
		by := map[string]string{}
		for _, g := range groups {
			by[g.Name] = g.Now
		}
		node := by["VPN"]
		for i := 0; i < 4 && by[node] != ""; i++ {
			node = by[node]
		}
		if node != "" {
			parts = append(parts, "через "+a.bold(node))
		}
	}
	if providers, err := m.Providers(ctx); err == nil {
		for _, p := range providers {
			if p.Info == nil || p.Info.Expire == 0 {
				continue
			}
			days := int(time.Until(time.Unix(p.Info.Expire, 0)).Hours() / 24)
			txt := fmt.Sprintf("подписка %s: %d дн.", p.Name, days)
			switch {
			case days <= 3:
				txt = a.red(txt)
			case days <= 7:
				txt = a.yellow(txt)
			}
			parts = append(parts, txt)
		}
	}
	return strings.Join(parts, " · ")
}

// newerVersion returns the newest release tag if it is newer than this program ("" otherwise).
func (a *App) newerVersion(ctx context.Context) string {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	rel, err := update.Latest(ctx, Repo)
	if err != nil || !update.Newer(rel.Tag, a.Version) {
		return ""
	}
	return rel.Tag
}
