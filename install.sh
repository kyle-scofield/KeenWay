#!/bin/sh
# KeenWay installer: on a Keenetic router with Entware (XKeen + mihomo) it installs the
# router build; on a Linux or macOS computer, the program that manages the router.
#   curl -fsSL https://raw.githubusercontent.com/scroge071-cloud/KeenWay/main/install.sh | sh
# Running it again updates KeenWay; settings in /opt/etc/keenway are kept.
set -e

REPO="${KEENWAY_REPO:-scroge071-cloud/KeenWay}"
BIN=/opt/sbin/keenway

# Colours only on a real terminal (with "curl | sh" the output still goes to the terminal).
if [ -t 1 ] && [ -z "$NO_COLOR" ]; then
  E=$(printf '\033')
  C() { printf '%s[1;38;2;%sm' "$E" "$1"; }
  R="$E[0m"; D="$E[2m"; G="$E[32m"; Y="$E[33m"; RED="$E[31m"
else
  C() { :; }
  R=; D=; G=; Y=; RED=
fi

say() { printf '%s\n' "$*"; }
die() { printf '%s✖%s Ошибка: %s\n' "$RED" "$R" "$*" >&2; exit 1; }

# The KeenWay logo: "Keen" white, "Way" violet, shading downwards — same as the app.
logo() {
  i=0
  while IFS='|' read -r keen way; do
    w=$((255 - i * 12)); v1=$((165 - i * 8)); v2=$((155 - i * 10)); v3=$((255 - i * 4))
    printf '  %s%s%s%s%s\n' "$(C "$w;$w;$w")" "$keen" "$(C "$v1;$v2;$v3")" "$way" "$R"
    i=$((i + 1))
  done <<'LOGO'
██╗  ██╗███████╗███████╗███╗   ██╗|██╗    ██╗ █████╗ ██╗   ██╗
██║ ██╔╝██╔════╝██╔════╝████╗  ██║|██║    ██║██╔══██╗╚██╗ ██╔╝
█████╔╝ █████╗  █████╗  ██╔██╗ ██║|██║ █╗ ██║███████║ ╚████╔╝
██╔═██╗ ██╔══╝  ██╔══╝  ██║╚██╗██║|██║███╗██║██╔══██║  ╚██╔╝
██║  ██╗███████╗███████╗██║ ╚████║|╚███╔███╔╝██║  ██║   ██║
╚═╝  ╚═╝╚══════╝╚══════╝╚═╝  ╚═══╝| ╚══╝╚══╝ ╚═╝  ╚═╝   ╚═╝
LOGO
  printf '  %sXKeen / mihomo без ручной правки конфигов · установка%s\n\n' "$D" "$R"
}

say ""
if [ "${COLUMNS:-80}" -ge 64 ] 2>/dev/null; then logo; else say "  $(C '236;238;247')Keen$(C '139;133;255')Way$R"; say ""; fi

# Where are we: a Keenetic router with Entware, or a computer?
if [ -d /opt/bin ] && command -v opkg >/dev/null 2>&1 && [ "$(uname -s)" = "Linux" ]; then
  kind=router
  command -v curl >/dev/null 2>&1 || die "нужен curl: opkg install curl"
  [ -d /opt/etc/mihomo ] || say "  ${Y}!${R} Не найден /opt/etc/mihomo — KeenWay работает с XKeen на ядре mihomo (xkeen -mihomo)."
  arch=$(opkg print-architecture 2>/dev/null | awk '{print $2}' | grep -v -e all -e noarch | tail -n 1)
  case "$arch" in
    mipsel*) target=linux-mipsle ;;
    mips*) target=linux-mips ;;
    aarch64*) target=linux-arm64 ;;
    armv7*) target=linux-armv7 ;;
    *)
      case "$(uname -m)" in
        aarch64) target=linux-arm64 ;;
        armv7*) target=linux-armv7 ;;
        *) die "неизвестная архитектура роутера: ${arch:-$(uname -m)}" ;;
      esac ;;
  esac
else
  # A computer: the same program manages the router over the network (terminal menu
  # or "keenway gui" for the window).
  kind=computer
  os=$(uname -s); m=$(uname -m)
  case "$os" in
    Linux) os=linux ;;
    Darwin) os=darwin ;;
    *) die "для Windows скачайте KeenWay-windows.exe со страницы https://github.com/$REPO/releases" ;;
  esac
  case "$m" in
    x86_64|amd64) m=amd64 ;;
    aarch64|arm64) m=arm64 ;;
    *) die "неизвестный процессор: $m" ;;
  esac
  target=$os-$m
  if [ "$(id -u)" = "0" ]; then
    BIN=/usr/local/bin/keenway
  else
    BIN="$HOME/.local/bin/keenway"
    mkdir -p "$HOME/.local/bin"
  fi
fi

old=""
[ -x "$BIN" ] && old=$("$BIN" version --plain 2>/dev/null || true)

url="https://github.com/$REPO/releases/latest/download/keenway-$target"
say "  Загрузка KeenWay для $target…"
tmp="$BIN.download"
curl -fL --retry 3 --progress-bar -o "$tmp" "$url" || die "не удалось скачать $url"
if curl -fsL -o "$tmp.sha256" "$url.sha256" 2>/dev/null; then
  want=$(awk '{print $1}' "$tmp.sha256")
  got=$(sha256sum "$tmp" | awk '{print $1}')
  rm -f "$tmp.sha256"
  [ "$want" = "$got" ] || { rm -f "$tmp"; die "контрольная сумма не совпала — файл повреждён"; }
  say "  ${G}✔${R} Контрольная сумма совпала"
fi
chmod 755 "$tmp"
mv -f "$tmp" "$BIN"

new=$("$BIN" version --plain 2>/dev/null || echo "?")
say ""
if [ -n "$old" ] && [ "$old" != "$new" ]; then
  say "  ${G}✔${R} KeenWay обновлён: $old → $new"
else
  say "  ${G}✔${R} KeenWay $new установлен"
fi
say ""
if [ "$kind" = computer ]; then
  say "  Управление роутером из терминала:  ${G}keenway${R}"
  say "  Окно программы:                    keenway gui"
  case ":$PATH:" in
    *":$(dirname "$BIN"):"*) ;;
    *) say ""; say "  ${Y}!${R} Папки $(dirname "$BIN") нет в PATH. Добавьте её или запускайте так: $BIN" ;;
  esac
else
  say "  Меню:          ${G}keenway${R}"
fi
say "  Все команды:   keenway help"
say "  Обновить:      keenway update"
say ""
