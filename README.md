# zakop

`zakop` - policy router для OpenWrt/ImmortalWrt, который принимает routing
решения до попадания трафика в `sing-box`.

**English version:** [README_EN.md](README_EN.md)

Главная идея: `nftables` решает, какой LAN client и какой destination нужно
отправить в proxy, а что должно идти `direct`. `sing-box` не получает весь
трафик подряд и не становится "главным router". Он используется только как:

- владелец `FakeIP` DNS;
- backend для `TProxy` inbound;
- executor для proxy outbounds.

Это принципиально отличается от схемы "весь LAN traffic -> sing-box". В zakop
обычный `direct` traffic остается вне sing-box.

## Status

Проект рассчитан на embedded-first установку на OpenWrt/ImmortalWrt.

Поддерживается:

- OpenWrt 23.05+
- OpenWrt 24.10+
- OpenWrt 25.12+
- ImmortalWrt 25.12+
- firewall4 / `nftables`
- IPv4 routing

Не поддерживается в v1:

- IPv6 routing;
- fw3 / iptables;
- custom transparent proxy внутри `zakopd`;
- custom FakeIP allocator;
- `.ipk` packaging.

## Requirements

Минимально:

- OpenWrt/ImmortalWrt с firewall4 / `nftables`;
- IPv4 LAN;
- `sing-box` package или compatible `sing-box` binary;
- 128 MB RAM;
- 25 MB free flash/overlay после установки зависимостей;
- 30 MB free `/tmp` на время install/upgrade.

Рекомендуется:

- 256 MB RAM или больше;
- 40 MB+ free flash/overlay, особенно если `sing-box` ставится как package;
- router class CPU уровня ARMv7/ARM64/MIPS 24Kc и выше.

Current embedded archive:

- download size: около 7 MB;
- unpacked install archive in `/tmp`: около 19 MB, потому что внутри binaries
  для нескольких CPU arch;
- installed zakop без `sing-box`: около 5 MB flash (`zakopd`, LuCI, scripts,
  config templates).

На устройствах с 64 MB RAM или 16 MB flash zakop обычно нецелесообразен:
`sing-box`, LuCI и provider caches быстро съедают запас. Для таких устройств
лучше использовать более лёгкую схему без `sing-box`.

## Install

Установка с GitHub:

```sh
sh -c "$(wget -O- https://raw.githubusercontent.com/elllkere/zakop/main/embedded/install.sh)"
```

или:

```sh
sh -c "$(curl -fsSL https://raw.githubusercontent.com/elllkere/zakop/main/embedded/install.sh)"
```

Installer скачивает archive из GitHub Releases:

```text
https://github.com/elllkere/zakop/releases/latest/download/zakop-openwrt-embedded.tar.gz
```

Если нужен свой mirror:

```sh
ZAKOP_BASE_URL='https://your-host/path' sh -c "$(wget -O- https://raw.githubusercontent.com/elllkere/zakop/main/embedded/install.sh)"
```

Для локального archive:

```sh
./embedded/install.sh --local ./dist/zakop-openwrt-embedded.tar.gz
```

## Миграция с neto

Новый installer умеет перенести существующую установку `neto` вместе с UCI,
provider caches и persistent data. Инструкции для автоматического переноса,
ручного восстановления и полностью чистой переустановки находятся в
[docs/MIGRATION_FROM_NETO.md](docs/MIGRATION_FROM_NETO.md).

## Upgrade

После установки:

```sh
/usr/share/zakop/upgrade.sh
```

`upgrade.sh` скачивает свежий installer из GitHub raw. Для своего installer URL:

```sh
ZAKOP_INSTALL_URL='https://your-host/install.sh' /usr/share/zakop/upgrade.sh
```

Способ проверки и загрузки релиза настраивается на странице General:
`update_via=direct|proxy`. Для `proxy` выбирается `update_outbound`; updater
поднимает временный локальный mixed proxy через выбранный sing-box outbound.
Трафик самого роутера при этом не добавляется в nft/TProxy policy.

## Uninstall

Обычное удаление оставляет `/etc/config/zakop` и `/etc/zakop`, чтобы можно было
поставить zakop заново без потери config:

```sh
/usr/share/zakop/uninstall.sh
```

Полное удаление вместе с config:

```sh
/usr/share/zakop/uninstall.sh --purge
```

Uninstall script останавливает service, убирает zakop-owned DNS/dnsmasq changes,
удаляет `zakopd`, LuCI files, `/usr/libexec/zakop`, `/usr/share/zakop`,
`/tmp/zakop`, `/var/lib/zakop` и DNS restore state under
`/etc/zakop/dnsmasq-state/`. Persistent provider cache under
`/etc/zakop/provider-cache/` is kept unless `--purge` is used.

## Quick Check

На router:

```sh
zakopd check
zakopd compile
/usr/libexec/zakop/sing-box check -c /tmp/zakop/sing-box.json
/etc/init.d/zakop restart
zakopd status
```

Проверка DNS:

```sh
dig @127.0.0.1 -p 5353 youtube.com A
dig @127.0.0.1 -p 5353 youtube.com AAAA
```

Проверка nft/TProxy:

```sh
nft list table inet zakop
ip -4 rule show
ip -4 route show table 101
```

## Routing Model

`zakop` работает только с LAN client traffic.

Порядок в generated `nft`:

1. LAN guard.
2. DNAT-associated connections, including port-forward replies, -> `return`.
3. `direct_clients4` -> `return`.
4. reserved/local destinations -> `return`.
5. `proxy_clients4` -> proxy.
6. `FakeIP` range -> proxy в `custom` mode.
7. Ordered IP/provider/CIDR rules.
8. default `return`.

`routing_mode`:

- `custom`: proxy только по rules/client policy/FakeIP/IP provider.
- `simple`: proxy по одному правилу из `config main` (`simple_*`), без
  автосоздания `config rule`.
- `global`: весь non-reserved LAN TCP/UDP идет в proxy, кроме direct clients.

Client policy:

- `default`: следует `routing_mode`;
- `proxy`: весь non-reserved TCP/UDP этого client идет в proxy; опциональный
  `outbound` выбирает custom sing-box outbound для этого client;
- `direct`: hard bypass, real DNS only, no FakeIP.

Rule action:

- `proxy`: отправить matching traffic в выбранный outbound;
- `direct`: вернуть traffic в обычный routing;
- `block`: для domain rules вернуть локальный DNS block response; для
  IP/CIDR/provider packet rules сгенерировать nft `drop`.

## DNS/FakeIP Semantics

DNS chain:

```text
LAN DNS -> dnsmasq -> zakopd -> selected local sing-box DNS listener
```

Packet chain:

```text
LAN traffic -> nft decides before sing-box
```

`zakopd` не реализует DoH/DoT/DoQ transport. Он только выбирает DNS path:

- `fakeip`;
- `real-direct`;
- `real-proxy`;
- `block`.

`sing-box` делает actual DNS transport: UDP, TCP, DoT, DoH.

Local DNS listeners:

- FakeIP: `127.0.0.1:15353`
- real-direct: `127.0.0.1:15354`
- real-proxy: `127.0.0.1:15355`

sing-box logs are kept out of OpenWrt `logread` / LuCI System Log. zakop starts
sing-box through a small wrapper that writes volatile logs to
`/tmp/zakop/sing-box.log`; LuCI shows it in the `Logs` page. The default path is
under `/tmp` to avoid persistent flash/overlay writes.

Semantics:

- domain `proxy` rules in `custom`/`simple` mode use `FakeIP`;
- provider/CIDR/IP rules use real DNS, потому что nft должен видеть real
  destination IP;
- direct rules and direct clients always use real DNS;
- `routing_mode=global` returns real DNS by default;
- AAAA для FakeIP domains возвращает NODATA при включенном
  `filter_aaaa_for_fakeip`.

Mixed rules разрешены:

```uci
config rule
	option action 'proxy'
	list domain_contains 'youtube'
	list ip_provider 'cloudflare_ipv4'
```

Это не AND между domain и IP:

- domain part работает только в DNS phase;
- provider/CIDR/IP part работает только в packet/nft phase;
- оба entry points используют один `action` и один `outbound`.

Port/proto matchers тоже packet-only:

```uci
list proto 'tcp'
list dst_port '443'
```

Ports не применяются к DNS/FakeIP domain matching.

## Outbounds

Built-in outbounds:

- `direct`
- `blocked`

Их не нужно создавать как `config outbound`.

Создаваемые outbound types:

- `vless`
- `hysteria2`
- `shadowsocks`
- `trojan`

Imports/subscriptions создают обычные outbound sections. После import node
можно выбрать в rule outbound dropdown так же, как manual profile.

Каждый proxy rule использует выбранный у него outbound независимо от других
rules. Пустых `Auto`/`Select outbound` в outbound selectors нет: новый rule
получает первый custom outbound, а без единого custom outbound LuCI не дает
создать proxy rule.

На той же странице можно создать priority pool из двух или более outbounds.
Порядок участников задает строгий приоритет: zakopd выбирает первый доступный,
переключается на следующий при отказе и автоматически возвращается на основной
после восстановления. Pool можно выбрать в rules, client policy, simple mode,
proxy DNS и для обновлений subscriptions, providers и самого zakop.

```uci
config outbound_pool 'main_pool'
	option label 'Main failover'
	list outbound 'primary'
	list outbound 'backup'
	option check_url 'https://www.gstatic.com/generate_204'
	option check_interval '60'
```

Кнопка `Test latency` на странице Outbounds проверяет все серверы через их
реальные proxy profiles с помощью URLTest самого sing-box. Результат выводится
в отдельном столбце таблицы Outbounds, лучший доступный сервер подсвечивается.
LuCI проверяет серверы отдельными последовательными запросами: таймаут одного
сервера не останавливает остальные. Для CLI доступна та же проверка:
`zakopd outbounds latency`.

## Providers

Provider - это data source, а rule - это routing policy.

Provider сам ничего не маршрутизирует. Он начинает влиять на routing только
после ссылки из rule:

```uci
list domain_provider 'youtube_domains'
list ip_provider 'cloudflare_ipv4'
```

Domain provider cache accepts one or more whitespace-separated domains per
line. Each domain entry is matched as the root domain and its subdomains, so
`x.com` matches both `x.com` and `api.x.com`.

LuCI Providers page кнопкой import добавляет provider presets, если provider с
таким URL или script path еще нет:

- Cloudflare IPv4: `https://www.cloudflare.com/ips-v4/`
- Telegram IPv4: `https://core.telegram.org/resources/cidr.txt`
- Akamai IPv4: `/usr/share/zakop/providers/akamai-ipv4.sh`
- AWS CDN IPv4 (`CLOUDFRONT`, `S3`): `/usr/share/zakop/providers/aws-ipv4.sh`
- AWS Full IPv4 (`AMAZON`, `EC2`, `GLOBALACCELERATOR`):
  `/usr/share/zakop/providers/aws-full-ipv4.sh`
- AWS Full EU IPv4: `/usr/share/zakop/providers/aws-full-eu-ipv4.sh`
- Google Cloud Europe IPv4: `/usr/share/zakop/providers/google-cloud-eu-ipv4.sh`

AWS Full can match broad AWS infrastructure and may affect ping to games hosted
on Amazon/AWS servers if a rule routes it through proxy.

Provider presets добавляются только для удобства. Они не создают rules и
создаются с `auto_update '0'`; включать автообновление пользователь решает сам.
Built-in JSON scripts используют `jq`, если он уже установлен, но не требуют
его: без `jq` работает POSIX fallback.

В секциях Subscriptions и Providers есть кнопка `Update all`: она сохраняет
текущую конфигурацию, обновляет все включенные подписки или все провайдеры и
перезапускает zakop. Индивидуальная кнопка `Update` в каждой секции остается
доступной. Обновления выполняются отдельными последовательными запросами с
прогрессом, поэтому длинный общий XHR не таймаутится; ошибка одного источника
не останавливает следующие.

Telegram feed содержит IPv6; zakop сохраняет только valid IPv4 CIDR/address
entries.

URL provider - дефолтный источник. Для feed в JSON или с лишними полями можно
использовать script provider: `type` всё ещё задаёт формат результата
(`domain`/`ip`), а `source 'script'` только меняет способ получения данных.
Скрипт должен вернуть по одному домену/IP/CIDR на строку: либо в stdout, либо
записав финальный результат в temp-файл из `ZAKOP_PROVIDER_OUTPUT`. zakop читает
этот файл только после завершения скрипта, сам нормализует результат, сохраняет
`/etc/zakop/provider-cache/<name>.txt` и обновляет metadata.

```uci
config provider 'json_ips'
	option label 'JSON IPs'
	option type 'ip'
	option source 'script'
	option script_path '/usr/share/zakop/providers/json-ips.sh'
	option auto_update '1'
	option update_schedule 'time'
	option update_hour '3'
	option update_minute '17'
```

Для автообновления по интервалу вместо фиксированного времени:

```uci
	option update_schedule 'interval'
	option update_interval_minutes '360'
```

Поддерживаемые интервалы: `15`, `30`, `60`, `120`, `180`, `360`, `720`,
`1440` минут.

Скрипту передаются `ZAKOP_PROVIDER_NAME`, `ZAKOP_PROVIDER_TYPE`,
`ZAKOP_PROVIDER_CACHE`, `ZAKOP_PROVIDER_OUTPUT` и другие `ZAKOP_PROVIDER_*`
переменные. При `update_via 'proxy'` zakop также выставляет
`HTTP_PROXY`/`HTTPS_PROXY`/`ALL_PROXY`.

Manual update:

On a fresh install, built-in provider names exist after importing provider
presets from the LuCI Providers page.

```sh
zakopd providers update
zakopd providers update telegram_ipv4
```

LuCI Providers page has an `Import provider presets` action that adds reusable
community domain sources and built-in IP URL/script sources. It only creates
providers, with `auto_update '0'`; rules are still configured separately.

Provider caches are written to `/etc/zakop/provider-cache/` so rules can compile
after an OpenWrt reboot even when `/var` is linked to volatile `/tmp`.
Legacy `local_path` values under `/var/lib/zakop/providers/` are treated as the
default provider cache and resolved to the persistent path.

If a referenced provider cache is missing, zakop logs a warning and compiles
that provider reference as empty until `zakopd providers update <name>` succeeds.

## LuCI

LuCI app находится в `Services -> zakop`.

Основные страницы:

- General: service status, DNS settings, routing mode.
- Outbounds: manual profiles, imports, subscriptions.
- Rules: domain/IP/provider rules, packet proto/ports.
- Clients: client policy.
- Providers: remote provider sources.
- Advanced: low-level DNS listeners, dnsmasq, LAN, TProxy, FakeIP range.
- Debug: `zakopd debug`.

В Rules page `dns_mode` скрыт и пишется как `auto`; DNS behavior выводится из
типа rule автоматически.

## Docs

- [Architecture](docs/ARCHITECTURE.md)
- [Router testing](docs/ROUTER_TESTING.md)
- [Outbounds](docs/OUTBOUNDS.md)
- [Testing](docs/TESTING.md)
- [Decisions](docs/DECISIONS.md)
- [Roadmap](docs/ROADMAP.md)
- [Release process](docs/RELEASE.md)

## Build

Локальная сборка:

```sh
GOCACHE=/tmp/zakop-go-cache go test ./...
GOCACHE=/tmp/zakop-go-cache ./embedded/pack.sh
./scripts/test-archive.sh
```

Archive:

```text
dist/zakop-openwrt-embedded.tar.gz
```

В archive должен быть top-level directory `zakop/`.

## Release Checklist

Чтобы install command из README работал с GitHub без отдельного домена:

1. Собрать archive:

```sh
GOCACHE=/tmp/zakop-go-cache ./embedded/pack.sh
./scripts/test-archive.sh
```

2. Создать GitHub Release.
3. Загрузить asset с точным именем:

```text
zakop-openwrt-embedded.tar.gz
```

Installer скачивает именно:

```text
https://github.com/elllkere/zakop/releases/latest/download/zakop-openwrt-embedded.tar.gz
```

Полный процесс: [docs/RELEASE.md](docs/RELEASE.md).
