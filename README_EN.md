# neto

`neto` is an OpenWrt/ImmortalWrt-first pre-sing-box policy router. Routing
decisions are made with firewall4/nftables before traffic reaches sing-box.
sing-box is used only as the FakeIP DNS owner, TProxy inbound backend, and proxy
outbound executor.

Russian README: [README.md](README.md)

## Install

```sh
sh -c "$(wget -O- https://raw.githubusercontent.com/elllkere/neto/main/embedded/install.sh)"
```

or:

```sh
sh -c "$(curl -fsSL https://raw.githubusercontent.com/elllkere/neto/main/embedded/install.sh)"
```

The installer downloads `neto-openwrt-embedded.tar.gz` from:

```text
https://github.com/elllkere/neto/releases/latest/download/
```

Override the archive base URL with `NETO_BASE_URL` when using a mirror.

Auto-update schedules default to fixed-time mode. Providers and subscriptions
can use interval mode with `option update_schedule 'interval'` and
`option update_interval_minutes '360'`. Supported intervals are `15`, `30`,
`60`, `120`, `180`, `360`, `720`, and `1440` minutes.

The LuCI Providers page can import provider presets. It includes community
domain provider sources for Telegram, TikTok, Twitter, YouTube, Meta, Discord,
and Anime from `itdoginfo/allow-domains`, plus built-in IP presets such as
Cloudflare, Telegram, Akamai, AWS, and Google Cloud scripts. This only creates provider
sections with `auto_update '0'`; rules are configured separately.

Domain provider cache accepts one or more whitespace-separated domains per
line. Each domain entry is matched as the root domain and its subdomains, so
`x.com` matches both `x.com` and `api.x.com`.

## Uninstall

Normal uninstall keeps `/etc/config/neto` and `/etc/neto` for later reinstall:

```sh
/usr/share/neto/uninstall.sh
```

Full removal including config:

```sh
/usr/share/neto/uninstall.sh --purge
```

The script stops the service, reverts neto-owned dnsmasq settings, and removes
installed neto binaries, LuCI files, runtime files, and DNS restore state under
`/etc/neto/dnsmasq-state/`. Provider cache under `/etc/neto/provider-cache/` is
kept unless `--purge` is used.

For the public GitHub install command to work, upload
`dist/neto-openwrt-embedded.tar.gz` as a GitHub Release asset named exactly:

```text
neto-openwrt-embedded.tar.gz
```

## Core Model

Bad model:

```text
LAN all traffic -> sing-box -> direct/proxy
```

neto model:

```text
LAN DNS -> dnsmasq -> netod -> selected sing-box DNS listener
LAN traffic -> nft decides before sing-box
```

Direct/bypass traffic must not enter sing-box.

Client `proxy` policy forces non-reserved TCP/UDP from that LAN client through
neto and selects a custom sing-box outbound for that client.

Each proxy rule independently uses its selected custom outbound. Outbound
selectors have no empty `Auto` choice: the first custom outbound is selected
initially, and LuCI does not create a proxy rule when none exists.

The Outbounds page can test every proxy profile and sort the results by HTTP
latency. The same JSON report is available with `netod outbounds latency`.

The page also supports strict-priority failover pools. netod checks members in
list order, selects the first reachable outbound, moves to the next member when
it fails, and automatically returns to the primary after recovery. Pools can be
selected by rules, proxy clients, simple mode, proxy DNS, and
subscription/provider/neto update settings.

```uci
config outbound_pool 'main_pool'
	option label 'Main failover'
	list outbound 'primary'
	list outbound 'backup'
	option check_url 'https://www.gstatic.com/generate_204'
	option check_interval '60'
```

## Status

Supported:

- OpenWrt 23.05+
- OpenWrt 24.10+
- OpenWrt 25.12+
- ImmortalWrt 25.12+
- firewall4/nftables
- IPv4 routing

Not in v1:

- IPv6 routing
- fw3/iptables
- transparent TCP/UDP proxy inside netod
- custom FakeIP allocator
- `.ipk` packaging

## Requirements

Minimum:

- OpenWrt/ImmortalWrt with firewall4 / nftables
- IPv4 LAN
- `sing-box` package or a compatible `sing-box` binary
- 128 MB RAM
- 25 MB free flash/overlay after dependencies
- 30 MB free `/tmp` during install/upgrade

Recommended:

- 256 MB RAM or more
- 40 MB+ free flash/overlay, especially when installing `sing-box` as a package
- ARMv7/ARM64/MIPS 24Kc-class router CPU or better

Current embedded archive is about 7 MB compressed and about 19 MB unpacked in
`/tmp` because it carries `netod` binaries for multiple CPU architectures.
Installed neto without `sing-box` takes about 5 MB of flash.

## DNS Semantics

- Domain proxy rules in `custom` and `simple` mode use FakeIP.
- Provider/CIDR/IP rules use real DNS so nftables can match real destination
  IPs.
- Direct rules and direct clients always use real DNS.
- `routing_mode=global` uses real DNS by default.
- Mixed domain + provider/CIDR/IP rules are allowed; domain matchers are DNS
  phase only, provider/CIDR/IP matchers are packet/nft phase only.
- `proto`, `src_port`, and `dst_port` are packet/nft phase only and never affect
  DNS/FakeIP matching.

## Logs

sing-box stdout/stderr is not forwarded to OpenWrt `logread` / LuCI System Log.
neto writes sing-box process logs to volatile `/tmp/neto/sing-box.log` and
exposes them on the LuCI `Logs` page. The default path is under `/tmp` to avoid
persistent flash/overlay writes.

## Docs

The main docs are currently maintained in Russian:

- [Architecture](docs/ARCHITECTURE.md)
- [Router testing](docs/ROUTER_TESTING.md)
- [Outbounds](docs/OUTBOUNDS.md)
- [Testing](docs/TESTING.md)
- [Decisions](docs/DECISIONS.md)
- [Roadmap](docs/ROADMAP.md)
- [Release process](docs/RELEASE.md)
