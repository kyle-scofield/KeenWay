package router

import (
	"errors"
	"slices"

	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"gopkg.in/yaml.v3"
	"path"
	"regexp"
	"strings"
	"time"
)

const (
	mihomoDir     = "/opt/etc/mihomo"
	configPath    = mihomoDir + "/config.yaml"
	newConfigPath = mihomoDir + "/config.keenway-new.yaml"
	backupDir     = mihomoDir + "/keenway-backups"
	rollbackFlag  = "/tmp/keenway-rollback"
	keepBackups   = 10
	// WatchdogSeconds is how long the router waits for the app to confirm before restoring the old config.
	WatchdogSeconds = 300
)

// Step reports progress of an apply to the UI.
type Step struct {
	ID   string `json:"id"` // connect, upload, activate, health, rollback, done
	Name string `json:"name"`
	OK   bool   `json:"ok"`
	Info string `json:"info,omitempty"`
}

// ApplyOptions controls an apply run.
type ApplyOptions struct {
	// Secret is the mihomo API password currently running on the router (to reload it).
	Secret string
	// NewSecret is the password in the new config (to check it after reload).
	NewSecret string
	// DryRun only uploads and validates; the running config is not touched.
	DryRun   bool
	Progress func(Step)
}

func (o ApplyOptions) report(id, name string, ok bool, info string) {
	if o.Progress != nil {
		o.Progress(Step{ID: id, Name: name, OK: ok, Info: info})
	}
}

// ErrTestNoMemory means "mihomo -t" was killed for lack of memory: the config itself may be
// fine. A second mihomo needs about as much memory as the running one, which a 256 MB
// router often doesn't have.
type ErrTestNoMemory struct{ FreeMB string }

func (e *ErrTestNoMemory) Error() string {
	return "проверке не хватило памяти роутера (свободно " + e.FreeMB + " МБ)"
}

// Validate uploads the config next to the live one and checks it with mihomo -t. The test
// runs with a tight Go memory limit; if it is still killed, page cache is dropped and it is
// tried once more before giving up with ErrTestNoMemory.
func (c *Conn) Validate(ctx context.Context, config []byte) error {
	sum := sha256.Sum256(config)
	script := fmt.Sprintf(`set -e
mkdir -p %[1]s
echo '%[2]s' | base64 -d > %[3]s
echo "%[4]s  %[3]s" | sha256sum -c - >/dev/null
set +e
kw_test() { GOGC=20 GOMEMLIMIT=40MiB mihomo -t -d %[5]s -f %[3]s >/tmp/keenway-test.log 2>&1; echo "kw_rc=$?"; }
kw_test
if ! grep -q "test is successful" /tmp/keenway-test.log && ! grep -qE "level=(error|fatal)" /tmp/keenway-test.log; then
  sync; echo 3 > /proc/sys/vm/drop_caches 2>/dev/null
  kw_test
fi
echo "kw_free=$(awk '/MemAvailable/{print int($2/1024)}' /proc/meminfo)"
tail -n 5 /tmp/keenway-test.log
`, backupDir, base64.StdEncoding.EncodeToString(config), newConfigPath, hex.EncodeToString(sum[:]), mihomoDir)
	out, err := c.Shell(ctx, script)
	if err != nil {
		return fmt.Errorf("загрузка на роутер: %v", err)
	}
	return checkTestOutput(out)
}

var kwFree = regexp.MustCompile(`kw_free=(\d+)`)

// checkTestOutput reads what the Validate script printed.
func checkTestOutput(out string) error {
	if strings.Contains(out, "test is successful") {
		return nil
	}
	if mihomoMsg.MatchString(out) {
		return &ConfigError{Reason: explainMihomo(out), Raw: lastLines(out, 3)}
	}
	// no verdict from mihomo at all: it was killed (out of memory) before finishing
	free := "?"
	if m := kwFree.FindStringSubmatch(out); m != nil {
		free = m[1]
	}
	return &ErrTestNoMemory{FreeMB: free}
}

// reloadScript makes mihomo use config.yaml. Reloading through the API keeps the old
// config in memory while the new one is built, which on a 128–256 MB router can starve it
// for minutes; when memory is short it restarts XKeen right away instead (a few seconds
// without proxy, but fast and it frees memory).
func reloadScript(secret string) string {
	auth := ""
	if secret != "" {
		auth = "-H " + shellQuote("Authorization: Bearer "+secret)
	}
	return fmt.Sprintf(`# XKeen builds its firewall rules (TProxy / Redirect / Hybrid) from the config only when it
# starts: if the config's inbounds no longer match the running rules, a reload is not enough.
kw_want=0; grep -qE '^redir-port:|type: *redir' %[2]s && kw_want=1
kw_have=0; iptables -t nat -S 2>/dev/null | grep xkeen_rule | grep -q REDIRECT && kw_have=1
echo "redir_config=$kw_want redir_rules=$kw_have"
avail=$(awk '/MemAvailable/{print int($2/1024)}' /proc/meminfo)
rss=0; for p in $(pidof mihomo); do r=$(awk '/VmRSS/{print int($2/1024)}' /proc/$p/status); [ "${r:-0}" -gt "$rss" ] && rss=$r; done
echo "mem=${avail}MB mihomo=${rss}MB"
code=skip
if [ "$kw_want" = "$kw_have" ] && [ "${avail:-0}" -gt $((rss + 48)) ]; then
  code=$(curl -s -o /tmp/keenway-reload.txt -w '%%{http_code}' -m 25 -X PUT %[1]s -H 'Content-Type: application/json' 'http://127.0.0.1:9090/configs?force=true' -d '{"path":"%[2]s"}') || true
fi
echo "reload_code=$code"
if [ "$code" != "204" ] && [ "$code" != "200" ]; then xkeen -restart </dev/null >/tmp/keenway-xkeen.log 2>&1; kw_how=restart; else kw_how=api; fi
echo "reload=$kw_how"
`, auth, configPath)
}

// groupNames lists the proxy groups a config defines; a running mihomo that has loaded
// the config shows all of them in /proxies.
func groupNames(config []byte) []string {
	var doc struct {
		Groups []struct {
			Name string `yaml:"name"`
		} `yaml:"proxy-groups"`
	}
	yaml.Unmarshal(config, &doc)
	var names []string
	for _, g := range doc.Groups {
		names = append(names, g.Name)
	}
	return names
}

// activate switches the running mihomo to config.yaml and waits, on the router itself
// (one SSH session — each one costs seconds on Keenetic), until every group of the new
// config is live. If the API reload did not take, it restarts XKeen and waits again.
func (c *Conn) activate(ctx context.Context, config []byte, o ApplyOptions) (string, error) {
	want := strings.Join(groupNames(config), "\n")
	var auths []string
	for _, sec := range []string{o.NewSecret, o.Secret, ""} {
		a := "-"
		if sec != "" {
			a = "Authorization: Bearer " + sec
		}
		if !slices.Contains(auths, a) {
			auths = append(auths, a)
		}
	}
	script := fmt.Sprintf(`echo '%[1]s' | base64 -d > /tmp/keenway-want.txt
loaded() {
  for a in %[2]s; do
    if [ "$a" = "-" ]; then curl -s -m 4 -o /tmp/keenway-proxies.json http://127.0.0.1:9090/proxies
    else curl -s -m 4 -H "$a" -o /tmp/keenway-proxies.json http://127.0.0.1:9090/proxies; fi
    grep -q '"proxies"' /tmp/keenway-proxies.json 2>/dev/null && break
  done
  while IFS= read -r n; do
    [ -z "$n" ] && continue
    grep -qF "\"$n\":" /tmp/keenway-proxies.json 2>/dev/null || return 1
  done < /tmp/keenway-want.txt
  return 0
}
wait_loaded() { i=0; while [ $i -lt $1 ]; do loaded && return 0; i=$((i+1)); sleep 1; done; return 1; }
%[3]s
if [ "$kw_how" = "api" ]; then
  if wait_loaded 20; then echo "result=ok"; exit 0; fi
  echo "retry=restart"
  xkeen -restart </dev/null >/tmp/keenway-xkeen.log 2>&1
fi
if wait_loaded 60; then echo "result=ok"; exit 0; fi
echo "result=fail"; tail -n 3 /tmp/keenway-xkeen.log
`, base64.StdEncoding.EncodeToString([]byte(want)), strings.Join(quoteAll(auths), " "), reloadScript(o.Secret))
	out, err := c.Shell(ctx, script)
	if err != nil {
		return "", fmt.Errorf("переключение не удалось: %v", err)
	}
	mem := between(out, "mem=", "\n")
	how := "mihomo перечитал конфиг"
	if strings.Contains(out, "reload=restart") || strings.Contains(out, "retry=restart") {
		how = "XKeen перезапущен"
	}
	if between(out, "result=", "\n") != "ok" {
		return "", fmt.Errorf("mihomo продолжает работать на старом конфиге (память: %s; перезапуск XKeen: %s)", mem, strings.TrimSpace(lastLines(out, 3)))
	}
	return how + " (память: " + mem + ")", nil
}

func quoteAll(list []string) []string {
	out := make([]string, len(list))
	for i, s := range list {
		out[i] = shellQuote(s)
	}
	return out
}

// Apply validates, backs up, activates and health-checks a new config, rolling back on failure.
// A watchdog on the router restores the old config by itself if the app never confirms.
func (c *Conn) Apply(ctx context.Context, config []byte, o ApplyOptions) error {
	// Clean-up and rollback must still run after the user pressed "Отменить".
	fresh := func() (context.Context, context.CancelFunc) {
		return context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
	}
	undo := func(secret string) {
		rctx, cancel := fresh()
		defer cancel()
		if rbErr := c.Rollback(rctx, secret); rbErr != nil && !strings.Contains(rbErr.Error(), "нет отложенного отката") {
			o.report("rollback", "Возврат прежнего конфига", false, rbErr.Error()+" — роутер вернёт его сам через 5 минут")
		} else {
			o.report("rollback", "Возврат прежнего конфига", true, "роутер работает на прежних настройках")
		}
	}
	canceled := func() bool { return ctx.Err() == context.Canceled }

	testNote := "mihomo принял новый конфиг"
	err := c.Validate(ctx, config)
	var noMem *ErrTestNoMemory
	if errors.As(err, &noMem) && !canceled() {
		// The config is still checked: mihomo loads it on activation, and if it fails the
		// previous config comes back automatically.
		testNote = "отдельная проверка не уместилась в памяти роутера (свободно " + noMem.FreeMB + " МБ) — конфиг проверится при включении, с автооткатом"
		err = nil
	}
	if err != nil {
		if canceled() {
			rctx, cancel := fresh()
			c.Shell(rctx, "rm -f "+newConfigPath)
			cancel()
			o.report("upload", "Загрузка и проверка конфига", false, "отменено")
			return ErrCanceled
		}
		o.report("upload", "Загрузка и проверка конфига", false, err.Error())
		return err
	}
	o.report("upload", "Загрузка и проверка конфига", true, testNote)
	if o.DryRun || canceled() {
		rctx, cancel := fresh()
		c.Shell(rctx, "rm -f "+newConfigPath)
		cancel()
		if canceled() {
			return ErrCanceled
		}
		return nil
	}

	activate := fmt.Sprintf(`set -e
ts=$(date +%%Y%%m%%d-%%H%%M%%S)
backup=%[1]s/config-$ts.yaml
cp %[2]s "$backup"
ls -t %[1]s/config-*.yaml 2>/dev/null | tail -n +%[3]d | xargs -r rm -f
echo "$backup" > %[4]s
mv %[5]s %[2]s
echo '%[7]s' | base64 -d > /tmp/keenway-reload.sh
( sleep %[6]d; if [ -f %[4]s ]; then cp "$(cat %[4]s)" %[2]s && rm -f %[4]s && sh /tmp/keenway-reload.sh; fi ) </dev/null >/dev/null 2>&1 &
echo "backup=$backup"
`, backupDir, configPath, keepBackups+1, rollbackFlag, newConfigPath, WatchdogSeconds,
		base64.StdEncoding.EncodeToString([]byte(reloadScript(o.NewSecret))))
	out, err := c.Shell(ctx, activate)
	if err != nil {
		if canceled() {
			o.report("activate", "Включение нового конфига", false, "отменено")
			undo(o.Secret)
			return ErrCanceled
		}
		o.report("activate", "Включение нового конфига", false, err.Error())
		return fmt.Errorf("не удалось применить: %v", err)
	}
	backup := between(out, "backup=", "\n")
	how, err := c.activate(ctx, config, o)
	if err != nil || canceled() {
		if canceled() {
			o.report("activate", "Включение нового конфига", false, "отменено")
			undo(o.Secret)
			return ErrCanceled
		}
		o.report("activate", "Включение нового конфига", false, err.Error())
		undo(o.Secret)
		return err
	}
	o.report("activate", "Включение нового конфига", true, how+"; прежний сохранён: "+path.Base(backup))

	warning, err := c.healthCheck(ctx, o.NewSecret)
	if err != nil || canceled() {
		if canceled() {
			o.report("health", "Проверка интернета и VPN", false, "отменено")
			undo(o.NewSecret)
			return ErrCanceled
		}
		o.report("health", "Проверка интернета и VPN", false, err.Error())
		undo(o.NewSecret)
		return err
	}
	if warning != "" {
		o.report("health", "Проверка интернета и VPN", true, "⚠️ "+warning)
	} else {
		o.report("health", "Проверка интернета и VPN", true, "сайты открываются, VPN отвечает")
	}
	if _, err := c.Shell(ctx, "rm -f "+rollbackFlag); err != nil {
		return fmt.Errorf("конфиг работает, но не удалось отменить автооткат: %v", err)
	}
	o.report("done", "Готово", true, "")
	return nil
}

// ErrCanceled is returned when the user stopped an apply; the router keeps (or got back) the old config.
var ErrCanceled = errors.New("применение отменено — роутер работает на прежних настройках")

// Rollback restores the backup recorded by the last Apply and reloads mihomo.
func (c *Conn) Rollback(ctx context.Context, secret string) error {
	_, err := c.Shell(ctx, fmt.Sprintf(`set -e
[ -f %[1]s ] || { echo "нет отложенного отката"; exit 1; }
cp "$(cat %[1]s)" %[2]s
rm -f %[1]s
%[3]s`, rollbackFlag, configPath, reloadScript(secret)))
	return err
}

// healthCheck waits for mihomo to come up with the new config, then checks plain internet
// and that the VPN group can reach the outside through at least one server.
func (c *Conn) healthCheck(ctx context.Context, secret string) (warning string, err error) {
	auth := ""
	if secret != "" {
		auth = "-H " + shellQuote("Authorization: Bearer "+secret)
	}
	// Right after a restart mihomo first downloads new rule lists, so give it time
	// instead of judging by the first attempt.
	script := fmt.Sprintf(`i=0; while [ $i -lt 30 ]; do
  v=$(curl -s -m 3 %[1]s http://127.0.0.1:9090/version)
  case "$v" in *version*) break;; esac
  i=$((i+1)); sleep 2
done
echo "api=$v"
k=0; code=000; while [ $k -lt 15 ]; do
  code=$(curl -s -o /dev/null -m 6 -w '%%{http_code}' -x http://127.0.0.1:1080 http://ya.ru)
  [ "$code" != "000" ] && break
  code=$(curl -s -o /dev/null -m 6 -w '%%{http_code}' -x http://127.0.0.1:1080 http://www.gstatic.com/generate_204)
  [ "$code" != "000" ] && break
  k=$((k+1)); sleep 2
done
echo "direct=$code"
j=0; while [ $j -lt 8 ]; do
  d=$(curl -s -m 10 %[1]s 'http://127.0.0.1:9090/proxies/VPN/delay?timeout=8000&url=http%%3A%%2F%%2Fwww.gstatic.com%%2Fgenerate_204')
  case "$d" in *delay*) break;; esac
  j=$((j+1)); sleep 2
done
echo "vpn=$d"
`, auth)
	hctx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()
	out, err := c.Shell(hctx, script)
	if err != nil {
		return "", fmt.Errorf("проверка не прошла: %v", err)
	}
	if !strings.Contains(between(out, "api=", "\n"), "version") {
		return "", fmt.Errorf("mihomo не запустился с новым конфигом")
	}
	if code := between(out, "direct=", "\n"); code == "000" || code == "" {
		return "", fmt.Errorf("пропал интернет (прямые сайты не открываются)")
	}
	// Dead VPN servers are the subscription's problem, not the config's: keep the config, warn.
	if !strings.Contains(between(out, "vpn=", "\n"), "delay") {
		return "интернет работает, но серверы VPN пока не отвечают — проверьте подписку на вкладке «Серверы»", nil
	}
	return "", nil
}

func between(s, start, end string) string {
	i := strings.Index(s, start)
	if i < 0 {
		return ""
	}
	s = s[i+len(start):]
	if j := strings.Index(s, end); j >= 0 {
		s = s[:j]
	}
	return strings.TrimSpace(s)
}

// Backups lists saved configs, newest first.
func (c *Conn) Backups(ctx context.Context) ([]string, error) {
	out, err := c.Shell(ctx, "ls -t "+backupDir+"/config-*.yaml 2>/dev/null")
	if err != nil {
		return nil, err
	}
	var list []string
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimSpace(l); strings.HasPrefix(l, backupDir) {
			list = append(list, l)
		}
	}
	return list, nil
}

// CurrentConfig downloads the live config.yaml.
func (c *Conn) CurrentConfig(ctx context.Context) ([]byte, error) {
	out, err := c.Shell(ctx, "base64 "+configPath)
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(strings.Join(strings.Fields(out), ""))
}

// TemplateUpdaterEnabled reports whether the rockblack-style cron job that overwrites groups and rules is active.
func (c *Conn) TemplateUpdaterEnabled(ctx context.Context) (bool, error) {
	out, err := c.Shell(ctx, `grep -v '^#' /opt/var/spool/cron/crontabs/root 2>/dev/null | grep -c 'update-config.sh' || true`)
	return strings.TrimSpace(lastLines(out, 1)) != "0" && strings.TrimSpace(out) != "", err
}

// DisableTemplateUpdater comments out that cron job so it can't overwrite the app's config.
func (c *Conn) DisableTemplateUpdater(ctx context.Context) error {
	_, err := c.Shell(ctx, `set -e
f=/opt/var/spool/cron/crontabs/root
cp "$f" "$f.keenway-bak"
sed -i '/^[^#].*update-config\.sh/s/^/# disabled by KeenWay: /' "$f"
killall -HUP crond 2>/dev/null || true`)
	return err
}

// ConfigError means mihomo refused the new config; nothing on the router changed.
type ConfigError struct {
	Reason string // in plain words
	Raw    string // what mihomo printed
}

func (e *ConfigError) Error() string { return e.Reason }

var mihomoMsg = regexp.MustCompile(`level=(?:error|fatal) msg="((?:[^"\\]|\\.)*)"`)

// explainMihomo turns the output of "mihomo -t" into a sentence a person can act on.
func explainMihomo(out string) string {
	msg := ""
	if m := mihomoMsg.FindAllStringSubmatch(out, -1); len(m) > 0 {
		msg = strings.ReplaceAll(m[len(m)-1][1], `\"`, `"`)
	} else {
		msg = lastLines(out, 2)
	}
	name := ""
	if i := strings.Index(msg, "]: "); i >= 0 {
		if j := strings.Index(msg[i+3:], ":"); j >= 0 {
			name = msg[i+3 : i+3+j]
		}
	}
	switch {
	case strings.Contains(msg, "duplicate provider name"), strings.Contains(msg, "duplicate group name"):
		return fmt.Sprintf("Имя «%s» занято дважды: так названы и подписка, и группа. Переименуйте подписку в разделе «Подключение».", name)
	case strings.Contains(msg, "not found"):
		return "В конфиге есть ссылка на то, чего нет: " + msg
	case strings.Contains(msg, "yaml"):
		return "Ошибка в записи конфига: " + msg
	}
	return "mihomo не принял конфиг: " + msg
}
