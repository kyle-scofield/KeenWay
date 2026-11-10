# KeenWay

Настройка XKeen (ядро mihomo) на роутерах Keenetic без ручной правки конфигов: вставил ключ или подписку, и получил рабочий конфиг. Дальше можно добавить свои сайты с иконками, правила для устройств, смотреть трафик и сколько дней осталось у подписки. Всё применяется с проверкой и автооткатом.


## Установка на роутер

Нужен Entware с XKeen на ядре mihomo. В терминале роутера (SSH):

```sh
curl -fsSL https://raw.githubusercontent.com/scroge071-cloud/KeenWay/main/install.sh | sh
```

Той же командой KeenWay обновляется (или `keenway update`). Удаление: `keenway uninstall`.

При первом запуске KeenWay читает текущий `/opt/etc/mihomo/config.yaml`: подписки, сервисы и ручные правила сохраняются.

## Как пользоваться

`keenway` без параметров открывает меню, пункты выбираются цифрами. То же самое можно сделать одной командой:

| Команда | Что делает |
|---|---|
| `keenway status` | VPN, текущий сервер, подписки: дни и трафик |
| `keenway add <ключ или ссылка>` | vless, hysteria2, trojan, ss, vmess, WireGuard/AmneziaWG, подписка https, happ://add |
| `keenway sub interval <имя> 6h` | как часто обновлять подписку (`30m`, `6h`, `1d`) |
| `keenway service youtube direct` | сервис напрямую / `vpn` / `on` / `off` |
| `keenway site add Кинопоиск kinopoisk.ru --group --icon https://…` | свой сайт, своя плитка в zashboard |
| `keenway device 192.168.1.50 vpn` | устройство всегда через VPN / `direct` / `block` / `default` / `pin` |
| `keenway servers test` · `keenway use 3` | пинг серверов, выбор сервера |
| `keenway traffic` | трафик по устройствам и сервисам |
| `keenway route web.telegram.org` | через что идёт сайт: правило, группа, сервер |
| `keenway profile vpn` | все плитки сразу: `normal` / `vpn` / `direct` / `games` |
| `keenway check` · `keenway apply` | проверить конфиг · применить на роутере |
| `keenway rollback` | вернуть прежний конфиг |
| `keenway password set` | пароль на zashboard (порт 9090) |
| `keenway updater off` | отключить `update-config.sh`, который перезаписывает правила |

Полный список: `keenway help`.

### Безопасное применение

1. Новый конфиг проверяется самим mihomo (`mihomo -t`). Если в нём ошибка, ничего не меняется.
2. Текущий конфиг копируется в `/opt/etc/mihomo/keenway-backups/` (хранятся последние 10).
3. mihomo перечитывает конфиг, после чего проверяются интернет и VPN.
4. Если проверка не прошла, сразу возвращается прежний конфиг. Если KeenWay прервался посреди применения, роутер сам вернёт прежний конфиг через 5 минут.

