package router

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
)

// Telegram notifications run on the router (cron, every 30 minutes), so they work
// while the computer is off. The script only reads the mihomo API.

const (
	notifyDir    = "/opt/etc/keenway"
	notifyScript = notifyDir + "/notify.sh"
	notifyConf   = notifyDir + "/notify.conf"
	cronFile     = "/opt/var/spool/cron/crontabs/root"
	cronTag      = "# keenway-notify"
)

// NotifyConfig is what the router script needs.
type NotifyConfig struct {
	Token  string // bot token from @BotFather
	ChatID string
	Days   int    // warn when a subscription has this many days left
	Secret string // mihomo panel password
	Name   string // how the router is called in messages
}

// notifySH is the router-side checker. Arithmetic is done in jq: busybox sh on MIPS
// has 32-bit integers, and traffic totals do not fit.
const notifySH = `#!/bin/sh
# KeenWay: Telegram notifications about subscriptions and servers.
. /opt/etc/keenway/notify.conf
STATE=/opt/etc/keenway/notify.state
touch "$STATE"
send() {
  for p in "-x http://127.0.0.1:1080" ""; do
    code=$(curl -s -m 20 $p -o /dev/null -w '%{http_code}' "https://api.telegram.org/bot$TOKEN/sendMessage" \
      --data-urlencode "chat_id=$CHAT" --data-urlencode "text=$1")
    [ "$code" = "200" ] && return 0
  done
  return 1
}
# once KEY TEXT: at most one message per key per day
once() {
  d=$(date +%Y-%m-%d)
  grep -qxF "$1 $d" "$STATE" && return 0
  send "$2" || return 1
  grep -vF "$1 " "$STATE" > "$STATE.tmp"; echo "$1 $d" >> "$STATE.tmp"; mv "$STATE.tmp" "$STATE"
}
has() { grep -qxF "flag $1" "$STATE"; }
set_flag() { has "$1" || echo "flag $1" >> "$STATE"; }
clear_flag() { grep -vxF "flag $1" "$STATE" > "$STATE.tmp"; mv "$STATE.tmp" "$STATE"; }

if [ "$1" = "test" ]; then
  send "✅ $NAME: уведомления KeenWay работают" && echo sent && exit 0
  echo "failed"; exit 1
fi

if [ -n "$SECRET" ]; then
  json=$(curl -s -m 15 -H "Authorization: Bearer $SECRET" http://127.0.0.1:9090/providers/proxies)
else
  json=$(curl -s -m 15 http://127.0.0.1:9090/providers/proxies)
fi
if ! echo "$json" | jq -e .providers >/dev/null 2>&1; then
  has mihomo || { send "⚠️ $NAME: mihomo не отвечает — VPN может не работать" && set_flag mihomo; }
  exit 0
fi
has mihomo && { send "✅ $NAME: mihomo снова работает"; clear_flag mihomo; }

now=$(date +%s)
echo "$json" | jq -r --argjson now "$now" '
  .providers | to_entries[] | .key as $name | .value | select(.vehicleType == "HTTP") | (.subscriptionInfo // {}) as $i |
  [ $name,
    (if ($i.Expire // 0) > 0 then ((($i.Expire - $now) / 86400) | floor) else -999 end),
    (if ($i.Total // 0) > 0 then (((($i.Upload // 0) + ($i.Download // 0)) * 100 / $i.Total) | floor) else -1 end),
    ([.proxies[]? | select(.alive == true)] | length),
    (.proxies | length),
    (if ($i.Expire // 0) > 0 then ($i.Expire | strftime("%d.%m.%Y")) else "-" end)
  ] | @tsv' | while IFS="$(printf '\t')" read -r name days pct alive count until; do
  if [ "$days" != "-999" ]; then
    if [ "$days" -lt 0 ]; then
      once "expired:$name" "❌ $NAME: подписка «$name» закончилась $until. Продлите её или вставьте новую ссылку в KeenWay."
    elif [ "$days" -le "$DAYS" ]; then
      once "expire:$name" "⏳ $NAME: подписка «$name» заканчивается через $days дн. (до $until)."
    fi
  fi
  if [ "$pct" -ge 90 ]; then
    once "traffic:$name" "📊 $NAME: по подписке «$name» израсходовано $pct% трафика."
  fi
  if [ "$count" -gt 0 ] && [ "$alive" -eq 0 ]; then
    has "down:$name" || { send "🔴 $NAME: ни один сервер подписки «$name» не отвечает." && set_flag "down:$name"; }
  elif has "down:$name"; then
    send "🟢 $NAME: серверы подписки «$name» снова работают ($alive из $count)." && clear_flag "down:$name"
  fi
done
`

func shellVar(name, value string) string { return name + "=" + shellQuote(value) + "\n" }

// InstallNotify puts the script and its settings on the router, adds the cron job
// and makes sure crond runs. With test it also sends a test message.
func (c *Conn) InstallNotify(ctx context.Context, cfg NotifyConfig, test bool) (string, error) {
	if cfg.Days <= 0 {
		cfg.Days = 3
	}
	if cfg.Name == "" {
		cfg.Name = "Роутер"
	}
	conf := shellVar("TOKEN", cfg.Token) + shellVar("CHAT", cfg.ChatID) + shellVar("DAYS", fmt.Sprint(cfg.Days)) +
		shellVar("SECRET", cfg.Secret) + shellVar("NAME", cfg.Name)
	b64 := base64.StdEncoding.EncodeToString
	script := fmt.Sprintf(`set -e
command -v jq >/dev/null || { echo "нужен jq: opkg install jq"; exit 1; }
mkdir -p %[1]s
umask 077
echo '%[2]s' | base64 -d > %[3]s
echo '%[4]s' | base64 -d > %[5]s
chmod 700 %[3]s; chmod 600 %[5]s
touch %[6]s
grep -v '%[7]s' %[6]s > %[6]s.tmp || true
echo '*/30 * * * * %[3]s %[7]s' >> %[6]s.tmp
cat %[6]s.tmp > %[6]s; rm -f %[6]s.tmp
if ! pidof crond >/dev/null; then
  for s in /opt/etc/init.d/S*crond /opt/etc/init.d/S*cron; do [ -x "$s" ] && "$s" start >/dev/null 2>&1 && break; done
  echo "crond=started"
fi
pidof crond >/dev/null && echo "crond=ok" || echo "crond=missing"
%[8]s
`, notifyDir, b64([]byte(notifySH)), notifyScript, b64([]byte(conf)), notifyConf, cronFile, cronTag, map[bool]string{true: notifyScript + " test || true", false: "echo sent"}[test])
	out, err := c.Shell(ctx, script)
	if err != nil {
		return "", fmt.Errorf("не удалось установить уведомления: %v", err)
	}
	switch {
	case strings.Contains(out, "crond=missing"):
		return "", fmt.Errorf("на роутере не запускается планировщик (crond) — уведомления не будут приходить")
	case !strings.Contains(out, "sent"):
		return "installed", fmt.Errorf("уведомления установлены, но тестовое сообщение не дошло: проверьте токен бота и chat id, и что вы написали боту /start")
	}
	if strings.Contains(out, "crond=started") {
		return "crond-started", nil
	}
	return "ok", nil
}

// RemoveNotify deletes the cron job and the script with its settings.
func (c *Conn) RemoveNotify(ctx context.Context) error {
	_, err := c.Shell(ctx, fmt.Sprintf(`[ -f %[1]s ] && { grep -v '%[2]s' %[1]s > %[1]s.tmp || true; cat %[1]s.tmp > %[1]s; rm -f %[1]s.tmp; }
rm -f %[3]s %[4]s %[5]s/notify.state
`, cronFile, cronTag, notifyScript, notifyConf, notifyDir))
	return err
}
