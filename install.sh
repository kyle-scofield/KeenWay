#!/bin/sh
# KeenWay installer for Keenetic routers with Entware (XKeen + mihomo).
#   curl -fsSL https://raw.githubusercontent.com/scroge071-cloud/KeenWay/main/install.sh | sh
# Running it again updates KeenWay; settings in /opt/etc/keenway are kept.
set -e

REPO="${KEENWAY_REPO:-scroge071-cloud/KeenWay}"
BIN=/opt/sbin/keenway

say() { printf '%s\n' "$*"; }
die() { printf 'Ошибка: %s\n' "$*" >&2; exit 1; }

[ -d /opt/bin ] || die "не найден Entware (/opt). Сначала установите Entware и XKeen."
command -v curl >/dev/null 2>&1 || die "нужен curl: opkg install curl"
[ -d /opt/etc/mihomo ] || say "Внимание: не найден /opt/etc/mihomo — KeenWay работает с XKeen на ядре mihomo (xkeen -mihomo)."

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

url="https://github.com/$REPO/releases/latest/download/keenway-$target"
say "Загрузка KeenWay ($target)…"
tmp="$BIN.download"
curl -fL --retry 3 -o "$tmp" "$url" || die "не удалось скачать $url"
if curl -fsL -o "$tmp.sha256" "$url.sha256" 2>/dev/null; then
  want=$(awk '{print $1}' "$tmp.sha256")
  got=$(sha256sum "$tmp" | awk '{print $1}')
  rm -f "$tmp.sha256"
  [ "$want" = "$got" ] || { rm -f "$tmp"; die "контрольная сумма не совпала — файл повреждён"; }
fi
chmod 755 "$tmp"
mv -f "$tmp" "$BIN"

say ""
say "Готово: $("$BIN" version)"
say "Запустите меню:   keenway"
say "Все команды:    keenway help"
