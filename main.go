// KeenWay configures XKeen/mihomo on Keenetic routers: paste a key or subscription,
// get a working config, apply it safely, watch traffic and subscriptions.
package main

import (
	"embed"
	"flag"
	"fmt"
	"golang.org/x/term"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"time"

	"keenway/internal/app"
	"keenway/internal/cli"
	"keenway/internal/store"
)

//go:embed web
var webFiles embed.FS

var version = "dev"

// onRouter reports whether this is the Keenetic router itself (Entware with mihomo).
func onRouter() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	if os.Getenv("KEENWAY_CLI") == "1" {
		return true
	}
	_, err := os.Stat("/opt/etc/mihomo")
	return err == nil
}

func main() {
	if onRouter() {
		// On the router: terminal program (menu or one-line commands), no window, no server.
		debug.SetMemoryLimit(24 << 20)
		st, err := store.OpenAt(filepath.Join(cli.StateDir, "state.json"))
		if err != nil {
			fmt.Fprintln(os.Stderr, "не удалось открыть настройки:", err)
			os.Exit(1)
		}
		if len(os.Args) > 1 && os.Args[1] == "serve" {
			servePanel(st)
			return
		}
		os.Exit(cli.New(st, version).Run(os.Args[1:]))
	}

	args := os.Args[1:]
	if !wantWindow(args) {
		// On a computer, in a terminal: the same menu and commands as on the router,
		// the router is reached over SSH. Settings are shared with the window version.
		st, err := store.Open()
		if err != nil {
			fmt.Fprintln(os.Stderr, "не удалось открыть настройки:", err)
			os.Exit(1)
		}
		enableTerminalColors()
		os.Exit(cli.NewRemote(st, version).Run(args))
	}
	if len(args) > 0 && (args[0] == "gui" || args[0] == "window") {
		args = args[1:]
	}
	flags := flag.NewFlagSet("keenway", flag.ExitOnError)
	noWindow := flags.Bool("no-window", false, "только запустить сервер и напечатать адрес")
	flags.Parse(args)

	st, err := store.Open()
	if err != nil {
		log.Fatalf("не удалось открыть настройки: %v", err)
	}
	ui, _ := fs.Sub(webFiles, "web")
	srv := app.New(st, ui)
	ln, url, err := srv.Listen()
	if err != nil {
		log.Fatalf("не удалось запустить: %v", err)
	}
	fmt.Println("KeenWay:", url)
	if !*noWindow {
		openWindow(url)
		// The window pings every few seconds; once it has been closed for a while, quit.
		go func() {
			time.Sleep(30 * time.Second)
			for range time.Tick(5 * time.Second) {
				if srv.Idle() > 45*time.Second {
					os.Exit(0)
				}
			}
		}()
	}
	log.Fatal(http.Serve(ln, srv.Handler()))
}

// servePanel runs the web panel on the router (started by /opt/etc/init.d/S99keenway-panel).
// It answers only the home network and asks for the panel password.
func servePanel(st *store.Store) {
	if st.Get().Panel.PassHash == "" {
		log.Fatal("пароль панели не задан: keenway panel on")
	}
	port := st.Get().Panel.Port
	if port == 0 {
		port = cli.DefaultPanelPort
	}
	ui, _ := fs.Sub(webFiles, "web")
	srv := app.NewPanel(st, ui)
	addr := fmt.Sprintf(":%d", port)
	log.Printf("KeenWay %s: панель на порту %d", version, port)
	server := &http.Server{Addr: addr, Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(server.ListenAndServe())
}

// wantWindow decides between the window and the terminal program on a computer:
// "keenway gui" or a launch without a terminal (double-click, Windows GUI build) opens
// the window; anything typed in a terminal runs the terminal program.
func wantWindow(args []string) bool {
	if len(args) > 0 {
		switch args[0] {
		case "gui", "window", "-no-window", "--no-window":
			return true
		}
		return false
	}
	return !term.IsTerminal(int(os.Stdin.Fd()))
}

// openWindow prefers a Chromium-based browser in "app" mode (own window, no address bar)
// with a separate profile; otherwise it opens the default browser.
func openWindow(url string) {
	profile := filepath.Join(os.TempDir(), "keenway-window")
	if dir, err := os.UserConfigDir(); err == nil {
		profile = filepath.Join(dir, "KeenWay", "window")
	}
	args := []string{"--app=" + url, "--user-data-dir=" + profile, "--window-size=1180,820", "--no-first-run", "--no-default-browser-check"}
	for _, b := range browserCandidates() {
		if path, err := exec.LookPath(b); err == nil {
			if err := exec.Command(path, args...).Start(); err == nil {
				return
			}
		}
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		fmt.Println("Откройте в браузере:", url)
	}
}

func browserCandidates() []string {
	if runtime.GOOS == "windows" {
		var out []string
		for _, env := range []string{"ProgramFiles(x86)", "ProgramFiles", "LocalAppData"} {
			base := os.Getenv(env)
			if base == "" {
				continue
			}
			out = append(out,
				filepath.Join(base, "Microsoft", "Edge", "Application", "msedge.exe"),
				filepath.Join(base, "Google", "Chrome", "Application", "chrome.exe"),
				filepath.Join(base, "Yandex", "YandexBrowser", "Application", "browser.exe"),
			)
		}
		return out
	}
	return []string{"chromium", "chromium-browser", "google-chrome-stable", "google-chrome", "microsoft-edge-stable", "brave-browser", "brave", "yandex-browser"}
}
