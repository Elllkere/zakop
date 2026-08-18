# Architecture

Этот документ описывает runtime architecture `zakop`. Термины `direct`,
`proxy`, `block`, `FakeIP`, `TProxy`, `provider`, `rule`, `outbound`,
`routing_mode` и UCI option names оставлены на английском, потому что это
реальные значения config/UI/CLI.

## Goal

`zakop` - pre-sing-box policy router для OpenWrt/ImmortalWrt.

Главное правило: routing decision должен происходить в `nftables` до того, как
traffic попадет в `sing-box`.

`sing-box` используется только как:

- владелец `FakeIP` DNS;
- backend для `TProxy` inbound;
- executor для proxy outbounds.

`zakopd` не должен становиться transparent TCP/UDP proxy.

## Supported Platforms

Поддерживается:

- OpenWrt 23.05+
- OpenWrt 24.10+
- OpenWrt 25.12+
- ImmortalWrt 25.12+
- firewall4 / `nftables`

Не поддерживается:

- fw3;
- iptables;
- OpenWrt старее 23.05;
- IPv6 routing в v1.

## Distribution

Основной формат поставки - embedded archive:

```text
dist/zakop-openwrt-embedded.tar.gz
```

Archive должен содержать top-level directory:

```text
zakop/
```

Installer:

- определяет OpenWrt/ImmortalWrt;
- выбирает `opkg` или `apk`;
- ставит зависимости;
- определяет CPU arch;
- ставит нужный `zakopd`;
- использует compatible system `sing-box`, если он есть;
- иначе ставит managed `sing-box` в `/usr/libexec/zakop/sing-box`;
- никогда не перезаписывает `/usr/bin/sing-box`.

Default archive URL:

```text
https://github.com/elllkere/zakop/releases/latest/download/zakop-openwrt-embedded.tar.gz
```

## Components

- `cmd/zakopd/main.go`: CLI entrypoint.
- `internal/config`: UCI parser и validation.
- `internal/ruleengine`: domain matcher и DNS decision logic.
- `internal/dnsproxy`: UDP/TCP DNS listener, DNS policy forwarding.
- `internal/provider`: remote provider update, IPv4 CIDR filtering/cache.
- `internal/policy`: IPv4 CIDR parse/normalize/dedup/collapse.
- `internal/nft`: nftables config generation.
- `internal/singbox`: sing-box config generation/check support.
- `internal/tproxy`: policy routing lifecycle.
- `internal/status`: status/debug checks.
- `embedded/files/etc/init.d/zakop`: procd service.
- `embedded/files/www/luci-static/resources/view/zakop`: LuCI app.

## Runtime Paths

- `/etc/config/zakop`
- `/etc/init.d/zakop`
- `/usr/bin/zakopd`
- `/usr/libexec/zakop/sing-box`
- `/usr/share/zakop/`
- `/etc/zakop/provider-cache/`
- `/etc/zakop/dnsmasq-state/`
- `/tmp/zakop/zakop.nft`
- `/tmp/zakop/sing-box.json`
- `/tmp/zakop/sing-box.log`
- `/var/lib/zakop/`

## Lifecycle

Start sequence:

1. `/etc/init.d/zakop` runs `zakopd check`.
2. Runs `zakopd compile`.
3. Runs `zakopd apply`.
4. Checks generated sing-box config.
5. Starts `zakopd run`.
6. Starts selected `sing-box run -c /tmp/zakop/sing-box.json`.
7. Configures dnsmasq integration and cron jobs.

Stop sequence:

- deletes zakop-owned nft table;
- removes zakop-owned `ip rule` / route table state;
- restores dnsmasq `server`, `noresolv`, `addsubnet`;
- removes zakop-owned cron block.

Stop/disable must not kill unrelated system `sing-box`.

## nft/TProxy Model

Generated table:

```text
table inet zakop
```

Order in `from_lan` chain:

1. DNAT-associated connections, including port-forward replies, -> `return`.
2. `direct_clients4` -> `return`.
3. `reserved4` -> `return`.
4. `proxy_clients4` -> first custom outbound TProxy target.
5. `FakeIP` range rule in `custom` mode when domain proxy rules exist.
6. ordered IP/provider/CIDR rules -> the selected outbound TProxy target.
7. default `return`.

Each selectable custom outbound or outbound pool has a generated TProxy inbound.
Concrete outbound ports come first in UCI order; pool ports are appended in UCI
order, so adding a pool does not renumber existing outbound listeners. Its port
is `tproxy_port + target_index`. nftables therefore preserves the selected
outbound or pool for packet-level IP/CIDR rules without expanding provider CIDRs
into sing-box rules.

An outbound pool is emitted as a sing-box `selector`. zakopd probes its concrete
members through the localhost-only Clash API in strict list order and selects
the first healthy member. It repeats the ordered check periodically, which also
returns a recovered higher-priority member to service. This management loop
does not proxy transparent traffic and does not add router-self nft rules.

FakeIP traffic initially enters the first TProxy inbound. Before sing-box route
matching, FakeIP reverse mapping restores the domain; generated ordered domain
route rules then select the outbound configured on the matching zakop rule.
Unmatched sing-box traffic has `route.final=direct`.

`TProxy` policy routing:

```sh
ip -4 rule add fwmark <mark> table <table>
ip -4 route add local default dev lo table <table>
```

Generated nft must be LAN-scoped. WAN, inbound, router self и non-LAN
prerouting traffic должны вернуться до proxy/TProxy rules.

## Routing Semantics

Client policy:

- absent/default: follows `routing_mode`;
- `proxy`: force non-reserved TCP/UDP traffic from client through zakop; optional
  `outbound` selects a custom sing-box outbound for that client;
- `direct`: hard bypass, real DNS only, no FakeIP.

`routing_mode`:

- `custom`: selective routing через ordered rules; unmatched traffic -> direct.
- `simple`: single-rule mode через `simple_*` fields in `config main`;
  regular `config rule` sections are not created or mutated.
- `global`: all LAN non-reserved TCP/UDP traffic -> proxy, кроме direct clients.

Rule action:

- `proxy`: route matching packet to selected outbound.
- `direct`: return before proxy.
- `block`: DNS phase returns local block response; packet phase drops matching
  IP/CIDR/provider traffic with nft `drop`.

## DNS/FakeIP

DNS path:

```text
LAN DNS -> dnsmasq -> zakopd -> selected sing-box DNS listener
```

Packet path:

```text
LAN traffic -> nft decides before sing-box
```

`zakopd` listens on `dns_listen`, by default:

```text
127.0.0.1:5353
```

`zakopd` is DNS policy forwarder only. It decides:

- `fakeip`;
- `real-direct`;
- `real-proxy`;
- `block`.

`sing-box` handles actual DNS transport:

- UDP DNS;
- TCP DNS;
- DoT;
- DoH.

Local sing-box DNS listeners:

- `singbox_dns_fakeip`: `127.0.0.1:15353`
- `singbox_dns_real_direct`: `127.0.0.1:15354`
- `singbox_dns_real_proxy`: `127.0.0.1:15355`

sing-box process logs are not forwarded to OpenWrt `logread`. `/etc/init.d/zakop`
starts sing-box through `/usr/share/zakop/run-sing-box-log.sh`, which writes
volatile logs to `/tmp/zakop/sing-box.log`; LuCI exposes that file through the
`Logs` page. The default path is under `/tmp` to avoid persistent flash/overlay
writes.

Real DNS config:

- `real_dns_mode`: `direct` or `proxy`
- `real_dns_outbound`: custom outbound for `real_dns_mode=proxy`
- `real_dns_transport`: `udp`, `tcp`, `tls`, `https`
- `real_dns_server`
- `real_dns_server_name`
- `real_dns_path`

`real_dns_upstream` and `dns_upstream_*` are legacy mirrors.

### DNS Rules

- Domain `proxy` rules in `custom`/`simple` mode use `FakeIP`.
- Provider/CIDR/IP rules use real DNS so nft can see real destination IP.
- Direct rules and direct clients always use real DNS.
- `routing_mode=global` returns real DNS by default.
- Block rules answer locally with NXDOMAIN/NODATA/block response.
- AAAA for FakeIP domains returns NODATA when `filter_aaaa_for_fakeip=1`.

dnsmasq uses `addsubnet=32` so `zakopd` can recover original LAN client IP
through EDNS Client Subnet. `zakopd` strips ECS before forwarding queries to
sing-box/public resolvers.

With `manage_dnsmasq=1`, the nftables table also redirects plain IPv4 LAN
TCP/UDP port 53 traffic to the router's dnsmasq. This covers clients behind an
AP/bridge which select a different classic DNS server. Encrypted DNS (DoH/DoT)
is not intercepted. At service startup, nft/TProxy and the dnsmasq upstream
switch are enabled only after an end-to-end real-DNS probe through zakopd and
sing-box succeeds.

Network and configuration reload triggers use rc.common's normal procd-aware
start path. They must not invoke `start_service()` directly: that bypasses
submission of the new procd instance set and can leave dnsmasq/nftables active
without their DNS backends.

## Rules and Matchers

Domain matchers are literal string operations:

- `domain_equals`: exact string match.
- `domain_contains`: substring match.
- `domain_starts_with`: prefix match.
- `domain_ends_with`: suffix match.

For root + subdomains use both:

```uci
list domain_equals 'example.com'
list domain_ends_with '.example.com'
```

Exclude fields use the same semantics:

- `exclude_domain_equals`
- `exclude_domain_contains`
- `exclude_domain_starts_with`
- `exclude_domain_ends_with`

IP/provider/CIDR matchers:

- `list ip_cidr '1.1.1.1'`
- `list ip_cidr '8.8.8.0/24'`
- `list ip_provider 'cloudflare_ipv4'`
- legacy `ip_file` / `file`

Protocol/port matchers:

- `list proto 'tcp'`
- `list proto 'udp'`
- `list src_port '1000-2000'`
- `list dst_port '443'`

Ports accept single value or `start-end`. If ports are set and `proto` is empty,
zakop generates explicit TCP and UDP rules.

Port/proto matchers are packet/nft-only. DNS/domain/FakeIP matching never sees
ports.

## Mixed Rules

Rules may mix domain matchers with provider/CIDR/IP matchers:

```uci
config rule
	option action 'proxy'
	option outbound 'my_vless'
	list domain_contains 'youtube'
	list ip_provider 'cloudflare_ipv4'
	list proto 'tcp'
	list dst_port '443'
```

This is not an AND between domain and IP.

- Domain part is DNS-phase only.
- Provider/CIDR/IP part is packet/nft-phase only.
- Port/proto narrows only the packet/nft part.
- Both entry points share one `action` and one `outbound`.

## Providers

Provider is data source only. Rule is routing policy only.

Provider types:

- `domain`
- `ip`

Providers download plain text lists into persistent storage:

```text
/etc/zakop/provider-cache/
```

OpenWrt `/var` may be backed by volatile `/tmp`, so default provider caches
must not live under `/var/lib/zakop/providers/`. Legacy `local_path` metadata
that points under `/var/lib/zakop/providers/` is treated as the default cache and
resolved to:

```text
/etc/zakop/provider-cache/
```

If a referenced provider cache is still missing, compile warns and treats that
provider reference as empty. Startup must continue so `zakopd` and `sing-box`
can run with the remaining valid policy.

Manual update:

On a fresh install, built-in provider names exist after importing provider
presets from the LuCI Providers page.

```sh
zakopd providers update
zakopd providers update telegram_ipv4
```

Provider source defaults to `url`. URL providers use `curl` to download raw
text. Domain provider text may contain one or more whitespace-separated domains
per line. A domain provider entry matches the root domain and subdomains.
Providers may also set `source=script` and an absolute `script_path`; the script
returns domains/IP/CIDRs on stdout or writes the final result to
`ZAKOP_PROVIDER_OUTPUT`, then `zakopd` normalizes and writes the standard provider
cache. Script providers keep `type=domain|ip`, because `type` describes the
output data consumed by rules.

Auto-update cron defaults to fixed time scheduling with `update_schedule=time`.
Provider fixed-time cron supports `update_hour` and `update_minute`. Missing
provider `update_minute` defaults to `5`, matching the old fixed minute.
Providers and subscriptions may instead use `update_schedule=interval` with
`update_interval_minutes` set to `15`, `30`, `60`, `120`, `180`, `360`, `720`,
or `1440`.

LuCI Providers can import provider presets if URL or script path is not already
present. Presets are convenience data sources only and must be created with
`auto_update=0`; users opt into scheduled updates themselves. The installer must
not create built-in provider sections automatically.

LuCI Providers can add community domain provider sources from itdoginfo
allow-domains. This creates only reusable `provider` sections with
`auto_update=0`; users still decide which rules reference them.

- Cloudflare IPv4: `https://www.cloudflare.com/ips-v4/`
- Telegram IPv4: `https://core.telegram.org/resources/cidr.txt`
- Akamai IPv4: `/usr/share/zakop/providers/akamai-ipv4.sh`
- AWS CDN IPv4 (`CLOUDFRONT`, `S3`): `/usr/share/zakop/providers/aws-ipv4.sh`
- AWS Full IPv4 (`AMAZON`, `EC2`, `GLOBALACCELERATOR`):
  `/usr/share/zakop/providers/aws-full-ipv4.sh`
- AWS Full EU IPv4: `/usr/share/zakop/providers/aws-full-eu-ipv4.sh`
- Google Cloud Europe IPv4: `/usr/share/zakop/providers/google-cloud-eu-ipv4.sh`

AWS Full is intentionally separate because routing broad AWS infrastructure may
affect ping to games hosted on Amazon/AWS servers.

Built-in JSON provider scripts use `jq` when it is already installed and fall
back to POSIX tools otherwise. `jq` is not a required zakop dependency.

IP provider update keeps only valid IPv4 address/CIDR entries. IPv6 entries are
ignored.

Creating a provider must not create a rule. Creating a rule must not create a
provider.

## Outbounds

Built-in outbound tags:

- `direct`
- `blocked`

They are always generated and must not be created as `config outbound`.

Creatable outbound types:

- `vless`
- `hysteria2`
- `shadowsocks`
- `trojan`

`proxy_default` is deprecated. LuCI must not create or offer it.

Imported nodes and subscription nodes are ordinary outbound sections and are
selectable by rules.

Outbound latency tests are management-only. `zakopd outbounds latency` starts
a temporary sing-box Clash API controller on localhost and queries the native
URLTest delay endpoint for each selected custom outbound. It discards a warm-up
pass, reports the measured pass, and exits after producing JSON. It does not
install nftables rules, create an automatic selector, or route router-self
traffic through the transparent proxy path.

## LuCI Layout

Tabs:

- General
- Outbounds
- Rules
- Clients
- Providers
- Advanced
- Debug

General contains service status, DNS settings, `routing_mode`, start/stop and
autostart. Advanced contains low-level listeners, dnsmasq, LAN, TProxy, FakeIP
range and nft settings.

Rules hides `dns_mode` and writes `auto`. DNS behavior is derived by zakopd.

## Forbidden Changes

- Do not add IPv6 routing in v1.
- Do not implement transparent TCP/UDP proxy in `zakopd`.
- Do not implement custom FakeIP allocator.
- Do not add fw3/iptables support.
- Do not route WAN/inbound/router-self/non-LAN prerouting traffic.
- Do not overwrite `/usr/bin/sing-box`.
- Do not make providers create rules or rules create providers.
- Do not generate thousands of nft rules for CIDR lists.
- Do not reintroduce `match_all`.
