# neto Agent Handoff

This repository contains `neto`, an embedded-first OpenWrt/ImmortalWrt service.
Future Codex sessions should read this file before making changes.

## Project Goal

`neto` is a pre-sing-box policy router for OpenWrt/ImmortalWrt. Routing
decisions must happen before traffic enters sing-box. Traffic that should be
direct/bypassed must never enter sing-box.

sing-box is used only as:

- FakeIP DNS owner.
- TProxy inbound backend.
- Proxy outbound executor.

`netod` must not become a transparent TCP/UDP proxy.

## Supported Targets

- OpenWrt 23.05
- OpenWrt 24.10
- OpenWrt 25.12+
- ImmortalWrt 25.12+ has been tested in development.

Only firewall4/nftables is supported. Do not add fw3/iptables support.

## Install Model

The project is embedded-first. Users should install without manually choosing
CPU architecture or binaries:

```sh
sh -c "$(wget -O- https://raw.githubusercontent.com/elllkere/neto/main/embedded/install.sh)"
sh -c "$(curl -fsSL https://raw.githubusercontent.com/elllkere/neto/main/embedded/install.sh)"
```

The embedded archive is `dist/neto-openwrt-embedded.tar.gz` and should contain a
top-level `neto/` directory.

## Runtime Paths

- `/etc/config/neto`
- `/etc/init.d/neto`
- `/usr/bin/netod`
- `/usr/libexec/neto/sing-box`
- `/usr/share/neto/`
- `/etc/neto/provider-cache/`
- `/tmp/neto/neto.nft`
- `/tmp/neto/sing-box.json`
- `/tmp/neto/sing-box.log`
- `/var/lib/neto/`

## Critical Invariants

- neto routes before sing-box.
- nftables decides packet routing before sing-box.
- direct/bypass traffic must never enter sing-box.
- sing-box owns FakeIP and the FakeIP domain mapping.
- sing-box stdout/stderr must not be forwarded to OpenWrt system log. The init
  script runs sing-box through `/usr/share/neto/run-sing-box-log.sh`, which
  writes volatile `/tmp/neto/sing-box.log` for the LuCI Logs page to avoid
  persistent flash/overlay writes.
- netod must not proxy transparent TCP/UDP traffic.
- neto must only route LAN client traffic.
- WAN, inbound, router self, and non-LAN prerouting traffic must return.
- DNAT-associated connections, including port-forward replies from LAN servers,
  must return before client proxy policy.
- IPv6 routing is not implemented in v1.
- AAAA for FakeIP-matched domains must not leak real IPv6 while IPv6 routing is
  absent.
- Large CIDR lists must compile into nft sets, not thousands of nft rules.
- Provider is a data source only.
- Rule is routing policy only.
- Creating a rule must not create a provider.
- Creating a provider must not create a rule.
- `match_all` is removed from v1.

## Current Routing Semantics

Client policies:

- absent/default: follow general `routing_mode`.
- `proxy`: force non-reserved TCP/UDP from this client through neto.
  `option outbound` selects a custom sing-box outbound for this client; a
  missing legacy value is normalized to the first custom outbound.
- `direct`: hard bypass. Real DNS only, no FakeIP, nft return before proxy rules.

Routing modes:

- `custom`: selective routing by ordered rules. Unmatched traffic follows
  `default_outbound`, currently only `direct`.
- `simple`: single-rule mode using `simple_*` fields in `config main`. It does
  not create or mutate `config rule` sections.
- `global`: all LAN client TCP/UDP goes to proxy except direct clients and
  reserved/local destinations.

## Current DNS Model

- `dns_listen` is the local netod DNS server/listener used by dnsmasq.
- netod is a DNS policy forwarder only. It must not implement normal-path DoH,
  DoT, or DoQ transport clients.
- sing-box handles DNS transport through three local DNS listeners:
  `singbox_dns_fakeip` (`127.0.0.1:15353`),
  `singbox_dns_real_direct` (`127.0.0.1:15354`), and
  `singbox_dns_real_proxy` (`127.0.0.1:15355`).
- `real_dns_mode` is `direct` or `proxy`.
- `real_dns_outbound` selects the custom outbound used by the real-proxy DNS
  listener when `real_dns_mode=proxy`; do not use `proxy_default`.
- `real_dns_transport` is `udp`, `tcp`, `tls`, or `https`.
- `real_dns_server` may be `host` or `host:port`; LuCI saves `host:port` using
  the default port for the selected transport when no port is entered.
- `real_dns_server_name` and `real_dns_path` describe the upstream used by
  generated sing-box DNS servers. LuCI combines these into one DoH server/path
  field for HTTPS.
- `real_dns_upstream` and `dns_upstream_*` are legacy compatibility mirrors.
- FakeIP DNS decisions forward DNS wire queries to sing-box FakeIP DNS.
  Direct/real DNS decisions forward DNS wire queries to the selected sing-box
  real DNS listener.
- DNS queries originating from loopback, a router-owned address, or any
  non-LAN source must always use real DNS and must not receive FakeIP. Router
  self traffic is excluded from nft/TProxy policy, so returning FakeIP to the
  router would make matched destinations unreachable.
- dnsmasq `addsubnet=32` is used only as local metadata so netod can recover
  the original LAN client IP. netod must strip EDNS Client Subnet before
  forwarding DNS queries to sing-box/public resolvers.
- When `manage_dnsmasq=1`, nftables redirects plain IPv4 LAN TCP/UDP port 53
  traffic to the router dnsmasq. This keeps DNS from clients behind a bridge/AP
  on the FakeIP policy path even when they select another classic DNS server.
  DoH/DoT are not intercepted.
- Domain proxy rules in custom and simple mode may use FakeIP. Direct clients,
  direct rules, provider/CIDR-only rules, and global mode use real DNS.
- A rule may mix domain matchers with provider/CIDR/IP matchers. Domain matchers
  are DNS-phase only; provider/CIDR/IP matchers are packet/nft-phase only. This
  is not an AND between domain and IP.

## Rule Matcher Semantics

Domain matchers are literal string operations, not DNS-aware matching.

- `domain_equals`: `normalizedDomain == normalizedValue`
- `domain_contains`: `strings.Contains(normalizedDomain, normalizedValue)`
- `domain_starts_with`: `strings.HasPrefix(normalizedDomain, normalizedValue)`
- `domain_ends_with`: `strings.HasSuffix(normalizedDomain, normalizedValue)`

Exclude fields use the same semantics:

- `exclude_domain_equals`
- `exclude_domain_contains`
- `exclude_domain_starts_with`
- `exclude_domain_ends_with`

Normalization:

- lowercase
- trim spaces
- trim trailing dot
- ignore empty values

For root + subdomains use both:

```uci
list domain_equals 'example.com'
list domain_ends_with '.example.com'
```

LuCI must not write deprecated matcher names:

- `domain_keyword`
- `domain_suffix`
- `domain_prefix`
- `domain_exact`

Rules can be filled from multiple LuCI input modes without changing matcher
semantics:

- domain fields: current `domain_*` and `exclude_domain_*` list fields
- domain textbox: writes the same `domain_*` and `exclude_domain_*` UCI lists
- domain providers: `list domain_provider`, selected from remote provider cache
- IP/CIDR list/textbox: `list ip_cidr`, IPv4 addresses become `/32`
- IP/CIDR providers: `list ip_provider`, selected from remote provider cache
- packet-only protocol/port matchers: `list proto 'tcp|udp'`, `list src_port`,
  and `list dst_port`; ports accept `443` or `1000-2000`
- local `domain_file`, `ip_file`, and legacy `file` are parser compatibility
  paths, not the primary LuCI UX

Protocol and port matchers only apply to provider/CIDR/IP nft rules. DNS/domain
FakeIP matching must ignore ports because DNS phase has no packet port.

## Current Provider Model

- Provider is a reusable remote data source.
- Provider types are `domain` and `ip`.
- Provider source is `url` by default. URL providers download plain text lists
  from `url` into
  `/etc/neto/provider-cache/`. Do not use `/var/lib/neto/providers/` as the
  default cache location because OpenWrt `/var` may be volatile.
- Provider source may be `script`. Script providers keep `type=domain|ip`,
  set `source=script`, and use an absolute `script_path`. The script returns
  one domain/IP/CIDR per line either on stdout or by writing the final result to
  the temp file path in `NETO_PROVIDER_OUTPUT`; netod reads that file only after
  the script exits. netod still normalizes, filters IPv4 for IP providers,
  writes the standard cache, and updates metadata. Scripts may use
  `NETO_PROVIDER_*` environment variables; with `update_via=proxy`, netod starts
  the temporary update proxy and exports `HTTP_PROXY`/`HTTPS_PROXY`/`ALL_PROXY`.
- Legacy `local_path` values under `/var/lib/neto/providers/` are treated as
  default provider cache paths and resolved to `/etc/neto/provider-cache/`.
- Missing provider caches must not abort compile/startup. neto should warn and
  compile that provider reference as empty until `netod providers update [name]`
  succeeds.
- `netod providers update [name]` updates URL providers using `curl` and script
  providers by executing `script_path`.
- LuCI Providers exposes `Update all`, which saves UCI and runs one named
  `netod providers update <name>` request at a time to avoid LuCI XHR timeouts.
  A failed provider is reported but must not stop later providers; restart neto
  once after the entire sequence.
- LuCI Providers page can import built-in IP provider presets for Cloudflare
  (`https://www.cloudflare.com/ips-v4/`), Telegram
  (`https://core.telegram.org/resources/cidr.txt`), Akamai
  (`/usr/share/neto/providers/akamai-ipv4.sh`), AWS CDN
  (`/usr/share/neto/providers/aws-ipv4.sh`), AWS Full
  (`/usr/share/neto/providers/aws-full-ipv4.sh`), and AWS Full EU
  (`/usr/share/neto/providers/aws-full-eu-ipv4.sh`), and Google Cloud Europe
  (`/usr/share/neto/providers/google-cloud-eu-ipv4.sh`). Installer must not create
  these provider sections automatically. Presets are convenience data sources
  only: they must be created with `auto_update=0` and no rules.
- IP provider updates save only valid IPv4 CIDR/address entries; IPv6 entries
  from mixed feeds such as Telegram are ignored.
- `auto_update=1` creates neto-owned cron entries, similar to protocol
  subscriptions. Fixed-time scheduling uses `update_schedule=time`.
  Providers support `update_hour` and `update_minute`; missing
  `update_minute` defaults to `5` for backward-compatible cron timing.
  Providers and subscriptions may instead use `update_schedule=interval` with
  `update_interval_minutes` set to `15`, `30`, `60`, `120`, `180`, `360`,
  `720`, or `1440`.
- `update_via=direct|proxy` follows the subscription update model and must not
  route router-self traffic through nftables.
- Rules reference providers with `domain_provider` or `ip_provider`.
- Creating or updating a provider must not create rules.
- LuCI Providers page may offer an import preset button for community domain
  sources and built-in IP URL/script sources. It must only create provider
  sections, with `auto_update=0`, and must not create rules.

## Current Outbound Model

- Built-in outbound tags are `direct` and `blocked`.
- Built-ins are generated for sing-box and must not be created as
  `config outbound` sections.
- Creatable outbound types are `vless`, `hysteria2`, `shadowsocks`, and
  `trojan`.
- Custom outbounds use stable UCI section/tag IDs plus editable `label`.
- Custom outbounds do not have an enable/disable switch in v1.
- Outbounds LuCI should keep the table compact: text label/name section title,
  type, address, port, and a read-only URLTest delay column. Do not add a second
  editable name/input column. Protocol details belong in the edit modal.
- Outbounds LuCI exposes a latency test. It invokes one named
  `netod outbounds latency <tag>` request at a time to avoid LuCI XHR timeouts,
  continues after an individual failure, and updates row results incrementally.
  The backend uses the native sing-box Clash API URLTest delay endpoint through
  a localhost-only temporary controller, without adding an automatic selector
  or router-self nft/TProxy routing.
- Outbounds LuCI should expose homeproxy-like controls for supported protocols:
  VLESS flow dropdown, Shadowsocks method dropdown, TLS min/max/ciphers, allow
  insecure, ECH, uTLS fingerprint, REALITY, and V2Ray transport fields. Hide
  REALITY public key/short ID until REALITY is enabled.
- `proxy_default` is deprecated. LuCI must not create or offer it.
- Old rules with `option outbound 'proxy_default'` may be normalized to
  the first custom outbound for compatibility.
- Outbound selectors do not expose `Auto` or an empty `Select outbound`
  choice. When a selection is missing, LuCI and config compatibility loading
  use the first custom outbound in UCI order.
- LuCI must not create a proxy rule when no custom outbound exists.
- Each proxy rule routes through its own selected outbound. IP/CIDR rules use
  per-outbound nft TProxy targets; FakeIP domain rules use sing-box reverse
  mapping followed by ordered domain route rules. Do not collapse rule
  outbounds into one shared sing-box `route.final`.
- `config outbound_pool` provides strict-priority failover across two or more
  unique concrete custom outbounds. Pool tags share the selectable namespace
  with outbound tags, nested pools are not supported, and list order is
  priority order. Pools are generated as sing-box selectors; netod uses the
  localhost-only Clash API to choose the first healthy member and return to a
  recovered higher-priority member. Pools are selectable everywhere a custom
  outbound is selected, including temporary update paths, which retry concrete
  pool members sequentially.
- Startup must not enable nft/TProxy policy or point dnsmasq at netod until an
  end-to-end query through netod and the selected sing-box real-DNS listener
  succeeds.
- Do not implement `reload_service()` by calling `start_service()` directly.
  Network/config reloads must use rc.common's procd-aware start wrapper so the
  service definition is actually submitted before `service_started()` runs.

## Current Import / Subscription Model

- Manual imports and subscriptions create normal `config outbound` sections.
- Imported nodes are selectable by rules through the same outbound tag dropdown
  as manual profiles.
- LuCI import and subscription management lives on the Outbounds page. Do not
  add a separate Imports tab unless the user explicitly asks for it.
- Imported outbound sections should carry `option imported '1'`; subscription
  nodes also carry `option subscription '<subscription_name>'`.
- Subscription nodes are ordinary editable outbounds in the main Outbounds
  table. A later update of the same subscription overwrites those nodes again.
- `config subscription '<name>'` supports `enabled`, `label`, `url`,
  `auto_update`, `update_schedule`, `update_hour`,
  `update_interval_minutes`, `update_via`, and `update_outbound`.
- Supported import URI schemes are `vless://`, `hysteria2://`/`hy2://`,
  `ss://`, and `trojan://`.
- `netod import-uri -file <path>` imports one or more share links from a local
  file.
- `netod subscriptions update [name]` downloads subscriptions and replaces only
  nodes belonging to that subscription.
- The Outbounds subscriptions section exposes `Update all`, which saves UCI and
  runs one named `netod subscriptions update <name>` request at a time to avoid
  LuCI XHR timeouts. A failed subscription must not stop later subscriptions;
  restart neto once after the entire sequence.
- Subscription downloads use the system `curl` binary, not Go `net/http`, to
  keep embedded multi-architecture `netod` binaries small.
- `auto_update=1` is implemented by neto-owned cron entries in
  `/etc/crontabs/root`; preserve user cron lines and only rewrite the marked
  neto block.
- `update_via=direct` uses direct curl fetching. `update_via=proxy` uses a
  temporary sing-box mixed inbound and a selected custom outbound; it must not
  route router-self traffic through nftables.
- The neto self-updater uses `config main` options `update_via=direct|proxy`
  and `update_outbound`. Proxy mode downloads the version marker, installer,
  and release archive through the same temporary sing-box mixed-proxy model;
  it must not add router-self nft/TProxy rules.
- Subscription update intervals are stored in UCI for scheduling/UX; manual
  update is currently the explicit LuCI action.

Do not assume a LuCI issue is fixed until it has been checked on an actual
OpenWrt/ImmortalWrt LuCI instance.

## Forbidden Changes

- Do not add IPv6 routing in v1.
- Do not implement a transparent TCP/UDP proxy in netod.
- Do not implement a custom FakeIP allocator in v1.
- Do not add fw3/iptables support.
- Do not route WAN/inbound/non-LAN prerouting traffic.
- Do not overwrite `/usr/bin/sing-box`.
- Do not make providers create rules or rules create providers.
- Do not generate thousands of nft rules for CIDR lists.
- Do not reintroduce `match_all`.

## Local Commands

```sh
go test ./...
sh -n embedded/*.sh scripts/*.sh embedded/files/usr/share/neto/*.sh embedded/files/usr/share/neto/providers/*.sh
sh -n embedded/files/usr/share/neto/providers/*.sh
jq empty embedded/files/usr/share/luci/menu.d/*.json
jq empty embedded/files/usr/share/rpcd/acl.d/*.json
node --check embedded/files/www/luci-static/resources/view/neto/*.js
./embedded/pack.sh
./scripts/test-archive.sh
```

If Go cache under `$HOME` is read-only in the sandbox, use:

```sh
GOCACHE=/tmp/neto-go-cache go test ./...
GOCACHE=/tmp/neto-go-cache ./embedded/pack.sh
```

## Documentation Lookup

When changing LuCI JavaScript under
`embedded/files/www/luci-static/resources/view/neto/`, query the current
OpenWrt LuCI documentation with Context7 before relying on `form` or `uci` API
details. Verify names and behavior for `form.Map`, sections, option flags,
save hooks, and `uci.sections`/`uci.set` persistence.

## Distribution Rebuild

After making repository changes, rebuild the embedded distribution archive and
validate it:

```sh
GOCACHE=/tmp/neto-go-cache ./embedded/pack.sh
./scripts/test-archive.sh
```
