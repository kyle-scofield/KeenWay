// Package app is the local HTTP server behind the KeenWay window.
package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
	"io/fs"
	"keenway/internal/core"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"keenway/internal/gen"
	"keenway/internal/links"
	"keenway/internal/mihomo"
	"keenway/internal/router"
	"keenway/internal/store"
	"keenway/internal/sub"
	"keenway/internal/update"
)

// Server serves the UI and the JSON API on 127.0.0.1 only.
type Server struct {
	st          *store.Store
	ui          fs.FS
	token       string
	addr        string
	mu          sync.Mutex
	password    string // router password for this session when not remembered
	lastPing    time.Time
	applying    sync.Mutex
	cancelApply context.CancelFunc // set while an apply runs
	// templateUpdater remembers whether the router's own config-overwriting cron job is active (nil = unknown).
	templateUpdater *bool
	// local is the web panel running on the router itself: commands run locally, access
	// is by the panel password from the home network instead of the window's one-time link.
	local   bool
	loginMu sync.Mutex
	// Version is this program's version and Repo the GitHub "owner/name" updates come from.
	Version, Repo string
	// Relaunch restarts the window program after an update (set by main).
	Relaunch func(path string)
}

// NewPanel is the server for the web panel on the router (keenway serve).
func NewPanel(st *store.Store, ui fs.FS) *Server {
	s := New(st, ui)
	s.local = true
	// the sign-in survives restarts and updates
	if t := st.Get().Panel.Session; t != "" {
		s.token = t
	} else {
		st.Update(func(x *store.State) { x.Panel.Session = s.token })
	}
	return s
}

func New(st *store.Store, ui fs.FS) *Server {
	b := make([]byte, 24)
	rand.Read(b)
	return &Server{st: st, ui: ui, token: hex.EncodeToString(b), lastPing: time.Now()}
}

// Listen binds a random local port and returns the URL the window should open.
func (s *Server) Listen() (net.Listener, string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, "", err
	}
	s.addr = ln.Addr().String()
	return ln, "http://" + s.addr + "/?t=" + s.token, nil
}

// Idle reports how long the window has been silent (used to exit after it closes).
func (s *Server) Idle() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return time.Since(s.lastPing)
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	api := map[string]func(http.ResponseWriter, *http.Request) (any, error){
		"GET /api/state":                s.getState,
		"POST /api/ping":                s.ping,
		"POST /api/router":              s.connectRouter,
		"GET /api/devices":              s.devices,
		"POST /api/devices/pin":         s.pinDevice,
		"POST /api/parse":               s.parse,
		"PUT /api/keys":                 s.putKeys,
		"POST /api/subscriptions/check": s.checkSubscription,
		"PUT /api/settings":             s.putSettings,
		"GET /api/preview":              s.preview,
		"POST /api/rollback":            s.rollback,
		"GET /api/status":               s.status,
		"POST /api/select":              s.selectNode,
		"POST /api/test":                s.testGroup,
		"POST /api/provider/update":     s.updateProvider,
		"POST /api/updater/disable":     s.disableUpdater,
		"GET /api/backups":              s.backups,
		"POST /api/secret/reveal":       s.revealSecret,
		"POST /api/route":               s.route,
		"POST /api/profile":             s.profile,
		"GET /api/backup":               s.backup,
		"POST /api/restore":             s.restore,
		"GET /api/notify":               s.getNotify,
		"PUT /api/notify":               s.putNotify,
		"POST /api/notify/chat":         s.findChat,
		"GET /api/version":              s.version,
		"GET /api/logs/router":          s.routerLogs,
		"POST /api/update":              s.selfUpdate,
	}
	for route, h := range api {
		mux.HandleFunc(route, s.guard(func(w http.ResponseWriter, r *http.Request) {
			v, err := h(w, r)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			if v != nil {
				writeJSON(w, http.StatusOK, v)
			}
		}))
	}
	mux.HandleFunc("POST /api/apply", s.guard(s.apply))
	mux.HandleFunc("GET /api/logs/live", s.guard(s.liveLogs))
	mux.HandleFunc("POST /api/apply/cancel", s.guard(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		cancel := s.cancelApply
		s.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": cancel != nil})
	}))
	static := http.FileServerFS(s.ui)
	if s.local {
		mux.HandleFunc("POST /login", s.login)
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if !s.hostOK(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if s.local {
			// The router's panel: everything but the icon needs the password.
			if !s.loggedIn(r) && r.URL.Path != "/icon.svg" {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Header().Set("Cache-Control", "no-store")
				fmt.Fprint(w, loginPage(""))
				return
			}
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src * data:; style-src 'self' 'unsafe-inline'")
			static.ServeHTTP(w, r)
			return
		}
		// The token in the first URL becomes a cookie; the page then sends it on every API call.
		if t := r.URL.Query().Get("t"); t != "" && subtle.ConstantTimeCompare([]byte(t), []byte(s.token)) == 1 {
			http.SetCookie(w, &http.Cookie{Name: "kw", Value: s.token, Path: "/", HttpOnly: false, SameSite: http.SameSiteStrictMode})
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src * data:; style-src 'self' 'unsafe-inline'")
		static.ServeHTTP(w, r)
	})
	return mux
}

// hostOK blocks DNS-rebinding: only our exact loopback address may be used as Host.
func (s *Server) hostOK(r *http.Request) bool {
	if s.local {
		return fromHomeNetwork(r)
	}
	return r.Host == s.addr
}

// fromHomeNetwork lets the router's panel answer only devices in private (home) networks,
// never the internet side.
func fromHomeNetwork(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast())
}

// loggedIn checks the session cookie of the router's panel.
func (s *Server) loggedIn(r *http.Request) bool {
	c, err := r.Cookie("kw")
	return err == nil && subtle.ConstantTimeCompare([]byte(c.Value), []byte(s.token)) == 1
}

// login checks the panel password; failed attempts are slowed down and serialized.
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if !s.hostOK(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	hash := s.st.Get().Panel.PassHash
	pw := r.PostFormValue("password")
	if hash == "" || bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) != nil {
		time.Sleep(2 * time.Second)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, loginPage("Неверный пароль"))
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "kw", Value: s.token, Path: "/", MaxAge: 30 * 24 * 3600, SameSite: http.SameSiteStrictMode})
	http.Redirect(w, r, "/", http.StatusFound)
}

func loginPage(errText string) string {
	msg := ""
	if errText != "" {
		msg = `<p class="err">` + errText + `</p>`
	}
	return `<!doctype html><html lang="ru"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>KeenWay</title><link rel="icon" href="icon.svg" type="image/svg+xml"><style>
:root{--bg:#f6f7fb;--card:#fff;--text:#14161f;--muted:#5d6377;--border:#dfe2ec;--accent:#5b54e8}
@media (prefers-color-scheme:dark){:root{--bg:#0e1018;--card:#171a26;--text:#eceef7;--muted:#9aa0bb;--border:#2a2f44;--accent:#8b85ff}}
*{box-sizing:border-box}body{margin:0;min-height:100vh;display:grid;place-items:center;background:var(--bg);color:var(--text);font:15px/1.45 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif;padding:16px}
form{background:var(--card);border:1px solid var(--border);border-radius:16px;padding:28px;width:min(360px,100%);display:flex;flex-direction:column;gap:14px}
h1{margin:0;font-size:1.4rem;display:flex;gap:10px;align-items:center}h1 span{color:var(--accent)}p{margin:0;color:var(--muted)}
input{font:inherit;padding:11px 12px;border-radius:10px;border:1px solid var(--border);background:var(--bg);color:var(--text)}
button{font:inherit;font-weight:600;padding:11px;border:0;border-radius:10px;background:var(--accent);color:#fff;cursor:pointer}.err{color:#e5484d}
</style></head><body><form method="post" action="/login">
<h1><img src="icon.svg" alt="" width="30" height="30">Keen<span>Way</span></h1>
<p>Панель роутера. Пароль задаётся командой <code>keenway panel password</code>.</p>` + msg + `
<input type="password" name="password" placeholder="Пароль панели" autocomplete="current-password" autofocus required>
<button>Войти</button></form></body></html>`
}

func (s *Server) guard(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.hostOK(r) || (s.local && !s.loggedIn(r)) || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-KeenWay")), []byte(s.token)) != 1 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		h(w, r)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func decode(r *http.Request, v any) error {
	if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 4<<20)).Decode(v); err != nil {
		return fmt.Errorf("неверный запрос: %v", err)
	}
	return nil
}

func ctxTimeout(r *http.Request, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), d)
}

// ---------- state ----------

func (s *Server) ping(http.ResponseWriter, *http.Request) (any, error) {
	s.mu.Lock()
	s.lastPing = time.Now()
	s.mu.Unlock()
	return map[string]bool{"ok": true}, nil
}

type catalogEntry struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Icon     string `json:"icon"`
	ViaVPN   bool   `json:"viaVPN"`
	Default  bool   `json:"default"`
	Category string `json:"category"`
	Includes string `json:"includes"`
}

func (s *Server) getState(_ http.ResponseWriter, r *http.Request) (any, error) {
	st := s.st.Get()
	if s.local {
		// links to zashboard etc. must use the address the browser already reaches the router by
		if h, _, err := net.SplitHostPort(r.Host); err == nil {
			st.Router.Host = h
		}
	}
	var catalog []catalogEntry
	for _, svc := range gen.Services {
		catalog = append(catalog, catalogEntry{svc.ID, svc.Name, svc.Icon, svc.ViaVPN, svc.Default, svc.Category, svc.Includes()})
	}
	imported := []map[string]any{}
	for _, raw := range st.RawProxies {
		if p, err := gen.ProxyFromYAML(raw); err == nil {
			imported = append(imported, map[string]any{"name": p.Name(), "type": p.Values["type"], "server": p.Values["server"]})
		}
	}
	settings := st.Settings
	hasSecret := settings.Secret != ""
	settings.Secret = ""
	return map[string]any{
		"router":          map[string]any{"host": st.Router.Host, "user": st.Router.User, "remembered": st.RouterPassword != "", "known": len(st.Router.HostKeys) > 0},
		"hasPassword":     s.routerPassword() != "",
		"settings":        settings,
		"hasSecret":       hasSecret,
		"keys":            append([]string{}, st.Keys...),
		"importedNodes":   imported,
		"catalog":         catalog,
		"profiles":        core.Profiles,
		"defaultExclude":  gen.DefaultAutoExclude,
		"imported":        st.Imported,
		"lastApplied":     st.LastApplied,
		"currentPending":  s.pendingChanges(st),
		"buildError":      s.buildError(st),
		"templateUpdater": s.updaterFlag(),
		"panel":           s.local,
	}, nil
}

// pendingChanges tells the UI whether the saved settings differ from what was last applied.
func (s *Server) pendingChanges(st store.State) bool {
	cfg, err := s.buildConfig(st)
	if err != nil {
		return false
	}
	return hash(cfg) != st.LastApplied
}

// buildError explains why the config can't be built ("" = fine). Without it the apply bar
// silently stayed hidden and a ticked service seemed to do nothing.
func (s *Server) buildError(st store.State) string {
	if _, err := s.buildConfig(st); err != nil {
		return err.Error()
	}
	return ""
}

func hash(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func (s *Server) updaterFlag() any {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.templateUpdater == nil {
		return nil
	}
	return *s.templateUpdater
}

func (s *Server) routerPassword() string {
	if s.local {
		return "local" // the panel runs on the router: no SSH password needed
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.password != "" {
		return s.password
	}
	return s.st.Get().RouterPassword
}

// ---------- router ----------

func (s *Server) connect(ctx context.Context) (*router.Conn, error) {
	if s.local {
		return router.Local(), nil
	}
	st := s.st.Get()
	pw := s.routerPassword()
	if pw == "" {
		return nil, fmt.Errorf("сначала подключитесь к роутеру (нужен пароль)")
	}
	t := st.Router
	t.Password = pw
	c, err := router.Connect(ctx, t)
	if err != nil {
		return nil, err
	}
	if c.NewHostKey != "" {
		s.st.Update(func(st *store.State) {
			if st.Router.HostKeys == nil {
				st.Router.HostKeys = map[string]string{}
			}
			st.Router.HostKeys[fmt.Sprint(c.Port)] = c.NewHostKey
		})
	}
	return c, nil
}

func (s *Server) connectRouter(_ http.ResponseWriter, r *http.Request) (any, error) {
	var req struct {
		Host     string `json:"host"`
		User     string `json:"user"`
		Password string `json:"password"`
		Remember bool   `json:"remember"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	if net.ParseIP(strings.TrimSpace(req.Host)) == nil {
		return nil, fmt.Errorf("укажите IP роутера, например 192.168.1.1")
	}
	ctx, cancel := ctxTimeout(r, 90*time.Second)
	defer cancel()
	changedHost := req.Host != s.st.Get().Router.Host
	s.st.Update(func(st *store.State) {
		st.Router.Host, st.Router.User = strings.TrimSpace(req.Host), strings.TrimSpace(req.User)
		if changedHost {
			st.Router.HostKeys = nil
		}
		if req.Remember {
			st.RouterPassword = req.Password
		} else {
			st.RouterPassword = ""
		}
	})
	s.mu.Lock()
	s.password = req.Password
	s.mu.Unlock()

	c, err := s.connect(ctx)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	out, _ := c.Shell(ctx, "command -v mihomo xkeen; mihomo -v 2>/dev/null | head -1")
	if !strings.Contains(out, "mihomo") {
		return nil, fmt.Errorf("подключение есть, но на роутере не найден XKeen с ядром mihomo")
	}
	resp := map[string]any{"port": c.Port, "cli": c.CLI, "mihomo": strings.TrimSpace(lastLine(out))}
	if mac, err := c.MAC(ctx); err == nil {
		s.st.Update(func(st *store.State) { st.RouterMAC = mac })
	}
	if on, err := c.TemplateUpdaterEnabled(ctx); err == nil {
		resp["templateUpdater"] = on
		s.mu.Lock()
		s.templateUpdater = &on
		s.mu.Unlock()
	}
	// First connection: read what the router already runs so nothing the user set up is lost.
	if !s.st.Get().Imported {
		live, err := c.CurrentConfig(ctx)
		if err == nil && len(live) > 0 {
			settings, warnings, err := gen.Import(live)
			if err == nil {
				var raw []string
				for _, p := range settings.Proxies {
					raw = append(raw, gen.ProxyToYAML(p))
				}
				settings.Proxies = nil
				s.st.Update(func(st *store.State) {
					st.Settings, st.RawProxies, st.Imported = settings, raw, true
					if cfg, err := s.buildConfig(*st); err == nil {
						st.LastApplied = hash(cfg)
					}
				})
				resp["imported"] = true
				resp["warnings"] = warnings
			}
		}
	}
	return resp, nil
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}

func (s *Server) devices(_ http.ResponseWriter, r *http.Request) (any, error) {
	ctx, cancel := ctxTimeout(r, 40*time.Second)
	defer cancel()
	c, err := s.connect(ctx)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	if !c.CLI {
		// Over Entware SSH the Keenetic CLI may be unavailable; the UI then asks for IPs manually.
		if devs, err := c.Devices(ctx); err == nil {
			return devs, nil
		}
		return []router.Device{}, nil
	}
	return c.Devices(ctx)
}

func (s *Server) pinDevice(_ http.ResponseWriter, r *http.Request) (any, error) {
	var req struct{ MAC, IP string }
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	ctx, cancel := ctxTimeout(r, 40*time.Second)
	defer cancel()
	c, err := s.connect(ctx)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	return map[string]bool{"ok": true}, c.PinIP(ctx, req.MAC, req.IP)
}

// ---------- keys, subscriptions, settings ----------

type parsedItem struct {
	Kind   string `json:"kind"`
	Name   string `json:"name,omitempty"`
	Type   string `json:"type,omitempty"`
	Server string `json:"server,omitempty"`
	URL    string `json:"url,omitempty"`
	Raw    string `json:"raw"`
	Error  string `json:"error,omitempty"`
}

func describe(items []links.Item) []parsedItem {
	var out []parsedItem
	for _, it := range items {
		p := parsedItem{Kind: it.Kind, URL: it.URL, Raw: it.Raw}
		if it.Err != nil {
			p.Error = it.Err.Error()
		}
		if it.Proxy != nil {
			p.Name, p.Type, p.Server = it.Proxy.Name(), fmt.Sprint(it.Proxy.Values["type"]), fmt.Sprint(it.Proxy.Values["server"])
		}
		out = append(out, p)
	}
	return out
}

func (s *Server) parse(_ http.ResponseWriter, r *http.Request) (any, error) {
	var req struct{ Text string }
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	return describe(links.Parse(req.Text)), nil
}

func (s *Server) putKeys(_ http.ResponseWriter, r *http.Request) (any, error) {
	var req struct {
		Keys       []string `json:"keys"`
		RawProxies []string `json:"rawProxies"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	for _, k := range req.Keys {
		for _, it := range links.Parse(k) {
			if it.Err != nil {
				return nil, fmt.Errorf("ключ не распознан: %v", it.Err)
			}
		}
	}
	return map[string]bool{"ok": true}, s.st.Update(func(st *store.State) {
		st.Keys = req.Keys
		if req.RawProxies != nil {
			st.RawProxies = req.RawProxies
		}
	})
}

// defaultHeaders makes subscription requests look like the router's own mihomo,
// so providers that bind to a device ID don't count the PC as an extra device.
func (s *Server) defaultHeaders() map[string]string {
	st := s.st.Get()
	h := map[string]string{"User-Agent": "mihomo/v1.19.30", "x-device-os": "Keenetic OS"}
	if st.RouterMAC != "" {
		h["x-hwid"] = st.RouterMAC
	}
	return h
}

func (s *Server) checkSubscription(_ http.ResponseWriter, r *http.Request) (any, error) {
	var req struct {
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	headers := req.Headers
	if len(headers) == 0 {
		headers = s.defaultHeaders()
	}
	res, err := sub.Fetch(r.Context(), req.URL, headers)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"nodes": res.Nodes, "format": res.Format, "errors": res.Errors, "hasInfo": res.HasInfo, "title": res.Info.Title}
	if res.HasInfo {
		out["used"], out["total"] = res.Info.Used(), res.Info.Total
		out["daysLeft"] = res.Info.DaysLeft(time.Now())
		if !res.Info.Expire.IsZero() {
			out["expire"] = res.Info.Expire.Unix()
		}
	}
	return out, nil
}

func (s *Server) putSettings(_ http.ResponseWriter, r *http.Request) (any, error) {
	var req struct {
		gen.Settings
		KeepSecret bool `json:"keepSecret"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	next := req.Settings
	for i := range next.Subscriptions {
		if len(next.Subscriptions[i].Headers) == 0 {
			next.Subscriptions[i].Headers = s.defaultHeaders()
		}
	}
	return map[string]bool{"ok": true}, s.st.Update(func(st *store.State) {
		if req.KeepSecret {
			next.Secret = st.Settings.Secret
		}
		st.Settings = next
	})
}

// buildConfig turns the saved state into config.yaml.
func (s *Server) buildConfig(st store.State) ([]byte, error) {
	settings := st.Settings
	settings.Proxies = nil
	for _, raw := range st.RawProxies {
		p, err := gen.ProxyFromYAML(raw)
		if err != nil {
			return nil, err
		}
		settings.Proxies = append(settings.Proxies, p)
	}
	for _, k := range st.Keys {
		for _, it := range links.Parse(k) {
			if it.Err != nil {
				return nil, fmt.Errorf("ключ не распознан: %v", it.Err)
			}
			if it.Proxy != nil {
				settings.Proxies = append(settings.Proxies, it.Proxy)
			}
		}
	}
	return gen.Build(settings)
}

func (s *Server) preview(w http.ResponseWriter, _ *http.Request) (any, error) {
	cfg, err := s.buildConfig(s.st.Get())
	if err != nil {
		return nil, err
	}
	// Hide credentials so the preview is safe to screenshot.
	text := string(cfg)
	if sec := s.st.Get().Settings.Secret; sec != "" {
		text = strings.ReplaceAll(text, sec, "••••••")
	}
	return map[string]string{"yaml": text}, nil
}

// ---------- apply ----------

// apply streams progress as server-sent events.
func (s *Server) apply(w http.ResponseWriter, r *http.Request) {
	if !s.applying.TryLock() {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "применение уже идёт"})
		return
	}
	defer s.applying.Unlock()
	dry := r.URL.Query().Get("dry") == "1"
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, _ := w.(http.Flusher)
	send := func(event string, v any) {
		b, _ := json.Marshal(v)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
		if flusher != nil {
			flusher.Flush()
		}
	}
	st := s.st.Get()
	cfg, err := s.buildConfig(st)
	if err != nil {
		send("done", map[string]any{"ok": false, "error": err.Error()})
		return
	}
	// Keep going even if the window is closed mid-apply; the router watchdog covers crashes.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	s.mu.Lock()
	s.cancelApply = cancel
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.cancelApply = nil; s.mu.Unlock() }()
	c, err := s.connect(ctx)
	if err != nil {
		send("step", router.Step{ID: "connect", Name: "Подключение к роутеру", OK: false, Info: err.Error()})
		send("done", map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer c.Close()
	send("step", router.Step{ID: "connect", Name: "Подключение к роутеру", OK: true, Info: st.Router.Host})
	running := s.liveSecret(ctx, st)
	err = c.Apply(ctx, cfg, router.ApplyOptions{
		Secret:    running,
		NewSecret: st.Settings.Secret,
		DryRun:    dry,
		Progress:  func(step router.Step) { send("step", step) },
	})
	if err != nil {
		done := map[string]any{"ok": false, "error": err.Error()}
		var ce *router.ConfigError
		if errors.Is(err, router.ErrCanceled) {
			done["canceled"] = true
		}
		if errors.As(err, &ce) {
			done["untouched"] = true // the router still runs the previous config
			done["details"] = ce.Raw
		}
		send("done", done)
		return
	}
	if !dry {
		s.st.Update(func(st *store.State) { st.LastApplied = hash(cfg) })
		// The notification script reads the mihomo API: keep its panel password current.
		if n := st.Notify; n.Enabled {
			c.InstallNotify(ctx, router.NotifyConfig{Token: n.Token, ChatID: n.ChatID, Days: n.Days, Secret: st.Settings.Secret, Name: "Роутер " + st.Router.Host}, false)
		}
	}
	if !dry && !s.local {
		// The router checks the internet from itself; this checks the path every other device
		// uses (traffic intercepted by XKeen), which can break while the router is fine.
		if warn := core.LANCheck(ctx); warn != "" {
			send("step", router.Step{ID: "lan", Name: "Проверка с этого компьютера", OK: true, Info: "⚠️ " + warn})
		} else {
			send("step", router.Step{ID: "lan", Name: "Проверка с этого компьютера", OK: true, Info: "сайты открываются через роутер"})
		}
	}
	send("done", map[string]any{"ok": true, "dry": dry})
}

// liveSecret returns the panel password the router runs now: the saved one if it works,
// otherwise none (a router that was never configured by the app).
func (s *Server) liveSecret(ctx context.Context, st store.State) string {
	for _, secret := range []string{st.Settings.Secret, ""} {
		if _, err := mihomo.New(s.mihomoHost(st), secret).Version(ctx); err == nil {
			return secret
		}
	}
	return st.Settings.Secret
}

func (s *Server) rollback(_ http.ResponseWriter, r *http.Request) (any, error) {
	ctx, cancel := ctxTimeout(r, 60*time.Second)
	defer cancel()
	c, err := s.connect(ctx)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	return map[string]bool{"ok": true}, c.Rollback(ctx, s.st.Get().Settings.Secret)
}

func (s *Server) backups(_ http.ResponseWriter, r *http.Request) (any, error) {
	ctx, cancel := ctxTimeout(r, 30*time.Second)
	defer cancel()
	c, err := s.connect(ctx)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	return c.Backups(ctx)
}

func (s *Server) disableUpdater(_ http.ResponseWriter, r *http.Request) (any, error) {
	ctx, cancel := ctxTimeout(r, 30*time.Second)
	defer cancel()
	c, err := s.connect(ctx)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	if err := c.DisableTemplateUpdater(ctx); err != nil {
		return nil, err
	}
	off := false
	s.mu.Lock()
	s.templateUpdater = &off
	s.mu.Unlock()
	return map[string]bool{"ok": true}, nil
}

// revealSecret shows the panel password; possessing the router login is enough to see it,
// which is the recovery path when the user forgets it.
func (s *Server) revealSecret(http.ResponseWriter, *http.Request) (any, error) {
	return map[string]string{"secret": s.st.Get().Settings.Secret}, nil
}

// ---------- logs ----------

// liveLogs streams mihomo's log as JSON lines until the page closes it.
func (s *Server) liveLogs(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	enc := json.NewEncoder(w)
	// a first line at once, so the page knows the stream is open
	enc.Encode(map[string]string{"level": "keenway", "message": "подключено к mihomo, ждём события…"})
	fl.Flush()
	err := s.mihomo(r.Context()).Logs(r.Context(), r.URL.Query().Get("level"), func(l mihomo.LogLine) {
		enc.Encode(l)
		fl.Flush()
	})
	if err != nil && r.Context().Err() == nil {
		enc.Encode(map[string]string{"level": "error", "message": "поток логов прервался: " + err.Error()})
		fl.Flush()
	}
}

// routerLogFiles are the journals worth reading on the router, newest knowledge first.
var routerLogFiles = []struct{ Path, Title string }{
	{"/tmp/keenway-test.log", "Последняя проверка конфига (mihomo -t)"},
	{"/tmp/keenway-xkeen.log", "Последний перезапуск XKeen"},
	{"/opt/var/log/xkeen-detached.log", "XKeen"},
	{"/opt/var/log/mihomo-update.log", "Обновление mihomo"},
	{"/opt/var/log/keenway-panel.log", "Веб-панель KeenWay"},
	{"/opt/var/log/keenway-update.log", "Обновление KeenWay"},
	{"/opt/var/log/keenway-notify.log", "Уведомления в Telegram"},
}

// routerLogs reads the router's log files (last 400 lines each) in one SSH session,
// plus memory and kernel messages about killed processes.
func (s *Server) routerLogs(_ http.ResponseWriter, r *http.Request) (any, error) {
	ctx, cancel := ctxTimeout(r, 40*time.Second)
	defer cancel()
	c, err := s.connect(ctx)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	var sb strings.Builder
	for i, f := range routerLogFiles {
		fmt.Fprintf(&sb, "echo '@@KW %d'; [ -f %s ] && tail -n 400 %s 2>/dev/null; ", i, f.Path, f.Path)
	}
	sb.WriteString(`echo '@@KW mem'; free 2>/dev/null; ps 2>/dev/null | grep -E '[m]ihomo|[x]ray|[k]eenway' ; `)
	sb.WriteString(`echo '@@KW kernel'; dmesg 2>/dev/null | grep -iE 'oom|killed process|out of memory|segfault' | tail -n 50`)
	out, err := c.Shell(ctx, sb.String())
	if err != nil && out == "" {
		return nil, err
	}
	type logFile struct {
		Title string   `json:"title"`
		Path  string   `json:"path"`
		Lines []string `json:"lines"`
	}
	files := []logFile{}
	var cur *logFile
	for _, line := range strings.Split(out, "\n") {
		if rest, ok := strings.CutPrefix(line, "@@KW "); ok {
			files = append(files, logFile{})
			cur = &files[len(files)-1]
			switch rest {
			case "mem":
				cur.Title, cur.Path = "Память и процессы", "free / ps"
			case "kernel":
				cur.Title, cur.Path = "Ядро: нехватка памяти и сбои", "dmesg"
			default:
				if i, err := strconv.Atoi(rest); err == nil && i < len(routerLogFiles) {
					cur.Title, cur.Path = routerLogFiles[i].Title, routerLogFiles[i].Path
				}
			}
			continue
		}
		if cur != nil && line != "" {
			cur.Lines = append(cur.Lines, line)
		}
	}
	for i := range files {
		if files[i].Lines == nil {
			files[i].Lines = []string{}
		}
	}
	return files, nil
}

// ---------- updates of KeenWay itself ----------

func (s *Server) version(_ http.ResponseWriter, r *http.Request) (any, error) {
	resp := map[string]any{"current": s.Version}
	rel, err := update.Latest(r.Context(), s.Repo)
	if err != nil {
		resp["error"] = err.Error()
		return resp, nil
	}
	resp["latest"], resp["url"], resp["notes"] = rel.Tag, rel.URL, rel.Notes
	resp["newer"] = update.Newer(rel.Tag, s.Version)
	return resp, nil
}

// selfUpdate installs the newest release. On the router the installer runs detached and
// restarts the panel; on a computer the program file is replaced and the window reopens.
func (s *Server) selfUpdate(_ http.ResponseWriter, r *http.Request) (any, error) {
	rel, err := update.Latest(r.Context(), s.Repo)
	if err != nil {
		return nil, err
	}
	if !update.Newer(rel.Tag, s.Version) {
		return nil, fmt.Errorf("уже установлена последняя версия (%s)", s.Version)
	}
	if s.local {
		url := "https://raw.githubusercontent.com/" + s.Repo + "/main/install.sh"
		script := "(sleep 2; curl -fsSL " + url + " | sh) >/opt/var/log/keenway-update.log 2>&1 &"
		if err := exec.Command("sh", "-c", script).Run(); err != nil {
			return nil, err
		}
		return map[string]any{"restarting": true, "to": rel.Tag}, nil
	}
	path, err := update.ReplaceSelf(r.Context(), s.Repo, rel.Tag, false)
	if err != nil {
		return nil, err
	}
	if s.Relaunch != nil {
		go func() {
			time.Sleep(time.Second)
			s.Relaunch(path)
		}()
	}
	return map[string]any{"restarting": true, "to": rel.Tag, "newWindow": true}, nil
}

// ---------- live status via the mihomo API ----------

// mihomoHost is where the mihomo API is: the router itself for the panel, the router's IP otherwise.
func (s *Server) mihomoHost(st store.State) string {
	if s.local {
		return "127.0.0.1"
	}
	return st.Router.Host
}

func (s *Server) mihomo(ctx context.Context) *mihomo.Client {
	st := s.st.Get()
	return mihomo.New(s.mihomoHost(st), s.liveSecret(ctx, st))
}

func (s *Server) status(_ http.ResponseWriter, r *http.Request) (any, error) {
	ctx, cancel := ctxTimeout(r, 20*time.Second)
	defer cancel()
	m := s.mihomo(ctx)
	version, err := m.Version(ctx)
	if err != nil {
		return map[string]any{"online": false, "error": err.Error()}, nil
	}
	groups, _ := m.Groups(ctx)
	providers, _ := m.Providers(ctx)
	traffic, _ := m.Traffic(ctx)
	out := map[string]any{"online": true, "version": version, "groups": groups, "providers": providers}
	// Subscriptions that are in the settings but not in the running mihomo: the router
	// still works with an older config (not applied yet, or mihomo did not reload).
	if cfg, err := s.buildConfig(s.st.Get()); err == nil {
		var doc struct {
			Providers map[string]any `yaml:"proxy-providers"`
		}
		if yaml.Unmarshal(cfg, &doc) == nil {
			running := map[string]bool{}
			for _, p := range providers {
				running[p.Name] = true
			}
			var missing []string
			for name := range doc.Providers {
				if !running[name] {
					missing = append(missing, name)
				}
			}
			out["notRunning"] = missing
		}
	}
	if traffic != nil {
		out["traffic"] = map[string]any{
			"upload": traffic.UploadTotal, "download": traffic.DownloadTotal, "connections": len(traffic.Connections),
			"byDevice": traffic.ByDevice(), "byService": traffic.ByService(),
		}
	}
	return out, nil
}

func (s *Server) selectNode(_ http.ResponseWriter, r *http.Request) (any, error) {
	var req struct{ Group, Option string }
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	ctx, cancel := ctxTimeout(r, 15*time.Second)
	defer cancel()
	return map[string]bool{"ok": true}, s.mihomo(ctx).Select(ctx, req.Group, req.Option)
}

func (s *Server) testGroup(_ http.ResponseWriter, r *http.Request) (any, error) {
	var req struct{ Group string }
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	ctx, cancel := ctxTimeout(r, 40*time.Second)
	defer cancel()
	return s.mihomo(ctx).TestGroup(ctx, req.Group)
}

func (s *Server) updateProvider(_ http.ResponseWriter, r *http.Request) (any, error) {
	var req struct{ Name string }
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	ctx, cancel := ctxTimeout(r, 60*time.Second)
	defer cancel()
	return map[string]bool{"ok": true}, s.mihomo(ctx).UpdateProvider(ctx, req.Name)
}

// route answers "which way does this site go": rule, group and server.
func (s *Server) route(_ http.ResponseWriter, r *http.Request) (any, error) {
	var req struct{ Query string }
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	host, port, err := core.ParseTarget(req.Query)
	if err != nil {
		return nil, err
	}
	ctx, cancel := ctxTimeout(r, 25*time.Second)
	defer cancel()
	return s.mihomo(ctx).Route(ctx, host, port)
}

func (s *Server) profile(_ http.ResponseWriter, r *http.Request) (any, error) {
	var req struct{ ID string }
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	ctx, cancel := ctxTimeout(r, 30*time.Second)
	defer cancel()
	n, err := core.ApplyProfile(ctx, s.mihomo(ctx), req.ID)
	return map[string]int{"changed": n}, err
}

// Backup is the settings file a user can keep or move to another computer.
// The router password is never included.
type Backup struct {
	App        string        `json:"app"`
	Version    int           `json:"version"`
	Router     router.Target `json:"router"`
	Settings   gen.Settings  `json:"settings"`
	Keys       []string      `json:"keys"`
	RawProxies []string      `json:"rawProxies,omitempty"`
	RouterMAC  string        `json:"routerMac,omitempty"`
}

func (s *Server) backup(w http.ResponseWriter, _ *http.Request) (any, error) {
	st := s.st.Get()
	w.Header().Set("Content-Disposition", `attachment; filename="keenway-settings.json"`)
	return Backup{App: "KeenWay", Version: 1, Router: st.Router, Settings: st.Settings, Keys: st.Keys, RawProxies: st.RawProxies, RouterMAC: st.RouterMAC}, nil
}

func (s *Server) restore(_ http.ResponseWriter, r *http.Request) (any, error) {
	var b Backup
	if err := decode(r, &b); err != nil {
		return nil, fmt.Errorf("файл не читается: %v", err)
	}
	if b.App != "KeenWay" || b.Version != 1 {
		return nil, fmt.Errorf("это не файл настроек KeenWay")
	}
	probe := b.Settings
	probe.Proxies = nil
	if len(probe.Subscriptions) == 0 && len(b.Keys) == 0 && len(b.RawProxies) == 0 {
		return nil, fmt.Errorf("в файле нет ни подписок, ни ключей")
	}
	return map[string]bool{"ok": true}, s.st.Update(func(st *store.State) {
		if b.Router.Host != "" {
			st.Router.Host, st.Router.User = b.Router.Host, b.Router.User
			if len(b.Router.HostKeys) > 0 {
				st.Router.HostKeys = b.Router.HostKeys
			}
		}
		st.Settings, st.Keys, st.RawProxies = b.Settings, b.Keys, b.RawProxies
		if b.RouterMAC != "" {
			st.RouterMAC = b.RouterMAC
		}
		st.Imported = true
	})
}

func maskToken(t string) string {
	if len(t) < 12 {
		return ""
	}
	return t[:6] + "…" + t[len(t)-4:]
}

func (s *Server) getNotify(http.ResponseWriter, *http.Request) (any, error) {
	n := s.st.Get().Notify
	return map[string]any{"enabled": n.Enabled, "token": maskToken(n.Token), "hasToken": n.Token != "", "chatId": n.ChatID, "days": max(n.Days, 3)}, nil
}

var tokenRe = regexp.MustCompile(`^\d{5,}:[A-Za-z0-9_-]{30,}$`)

// putNotify saves the settings and installs (or removes) the checker on the router.
func (s *Server) putNotify(_ http.ResponseWriter, r *http.Request) (any, error) {
	var req struct {
		Enabled bool   `json:"enabled"`
		Token   string `json:"token"` // empty = keep the saved one
		ChatID  string `json:"chatId"`
		Days    int    `json:"days"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	st := s.st.Get()
	n := st.Notify
	if t := strings.TrimSpace(req.Token); t != "" {
		if !tokenRe.MatchString(t) {
			return nil, fmt.Errorf("это не похоже на токен бота (вид 123456789:AA…)")
		}
		n.Token = t
	}
	n.ChatID, n.Days, n.Enabled = strings.TrimSpace(req.ChatID), max(1, min(req.Days, 30)), req.Enabled
	if n.Enabled && n.Token == "" {
		return nil, fmt.Errorf("вставьте токен Telegram-бота")
	}
	ctx, cancel := ctxTimeout(r, 60*time.Second)
	defer cancel()
	if n.Enabled && n.ChatID == "" {
		id, _, err := lookupChat(ctx, n.Token)
		if err != nil {
			return nil, err
		}
		n.ChatID = id
	}
	c, err := s.connect(ctx)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	result := "removed"
	if n.Enabled {
		result, err = c.InstallNotify(ctx, router.NotifyConfig{Token: n.Token, ChatID: n.ChatID, Days: n.Days, Secret: st.Settings.Secret, Name: "Роутер " + st.Router.Host}, true)
		if err != nil && result == "" {
			return nil, err
		}
	} else if err = c.RemoveNotify(ctx); err != nil {
		return nil, err
	}
	if uerr := s.st.Update(func(st *store.State) { st.Notify = n }); uerr != nil {
		return nil, uerr
	}
	if err != nil {
		return nil, err // installed, but the test message failed
	}
	return map[string]string{"result": result}, nil
}

// findChat reads the bot's recent messages to get the chat id of whoever wrote /start.
func (s *Server) findChat(_ http.ResponseWriter, r *http.Request) (any, error) {
	var req struct{ Token string }
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	token := strings.TrimSpace(req.Token)
	if token == "" {
		token = s.st.Get().Notify.Token
	}
	if !tokenRe.MatchString(token) {
		return nil, fmt.Errorf("сначала вставьте токен бота")
	}
	ctx, cancel := ctxTimeout(r, 20*time.Second)
	defer cancel()
	id, name, err := lookupChat(ctx, token)
	if err != nil {
		return nil, err
	}
	return map[string]string{"chatId": id, "name": name}, nil
}

// lookupChat reads the bot's recent messages to get the chat of whoever wrote to it.
func lookupChat(ctx context.Context, token string) (string, string, error) {
	req2, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.telegram.org/bot"+token+"/getUpdates?limit=20", nil)
	resp, err := http.DefaultClient.Do(req2)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err // the URL holds the token: never show it
		}
		return "", "", fmt.Errorf("Telegram недоступен: %v", err)
	}
	defer resp.Body.Close()
	var out struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
		Result      []struct {
			Message *struct {
				Chat struct {
					ID        int64  `json:"id"`
					FirstName string `json:"first_name"`
					Username  string `json:"username"`
					Title     string `json:"title"`
				} `json:"chat"`
			} `json:"message"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", "", err
	}
	if !out.OK {
		return "", "", fmt.Errorf("Telegram: %s", out.Description)
	}
	for i := len(out.Result) - 1; i >= 0; i-- {
		if m := out.Result[i].Message; m != nil {
			name := m.Chat.FirstName
			if m.Chat.Title != "" {
				name = m.Chat.Title
			}
			if m.Chat.Username != "" {
				name += " (@" + m.Chat.Username + ")"
			}
			return strconv.FormatInt(m.Chat.ID, 10), name, nil
		}
	}
	return "", "", fmt.Errorf("бот пока не получил от вас сообщений — откройте бота в Telegram, нажмите «Старт» (/start) и попробуйте ещё раз")
}
