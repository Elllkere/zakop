# Router Testing

Команды ниже выполняются на OpenWrt/ImmortalWrt router после установки `zakop`.

Technical terms (`direct`, `proxy`, `block`, `FakeIP`, `TProxy`, `provider`,
`outbound`) оставлены как в UCI/LuCI/CLI.

## Install Check

```sh
zakopd version
zakopd check
zakopd compile
/usr/libexec/zakop/sing-box check -c /tmp/zakop/sing-box.json
/etc/init.d/zakop restart
zakopd status
```

Если используется system `sing-box`:

```sh
sing-box check -c /tmp/zakop/sing-box.json
```

Logs:

```sh
logread | grep -E 'zakopd|sing-box|dnsmasq' | tail -n 120
```

## Routing Semantics

`routing_mode`:

- `custom`: ordered `config rule` decide `proxy`/`direct`/`block`.
- `simple`: one rule from `config main` `simple_*` fields decides
  `proxy`/`direct`/`block`.
- `global`: all LAN non-reserved TCP/UDP traffic goes to proxy except direct
  clients.

Client policy:

- `default`: follows `routing_mode`;
- `proxy`: forces non-reserved traffic from this client through zakop;
- `direct`: hard bypass, real DNS only, no FakeIP.

Rules are evaluated by ascending `priority`.

## Domain Matchers

Domain matchers are literal string operations:

- `domain_equals youtube.com` matches only `youtube.com`.
- `domain_contains youtube` matches `youtube.com`, `youtube.kz`,
  `notyoutube.com`.
- `domain_starts_with you` matches `youtube.com`.
- `domain_ends_with youtube.com` matches `youtube.com`, `www.youtube.com`,
  `notyoutube.com`.
- `domain_ends_with .youtube.com` matches `www.youtube.com`, but not
  `youtube.com`.

For root + subdomains:

```uci
list domain_equals 'youtube.com'
list domain_ends_with '.youtube.com'
```

Exclude fields:

- `exclude_domain_equals`
- `exclude_domain_contains`
- `exclude_domain_starts_with`
- `exclude_domain_ends_with`

## Providers

Provider is data source only. It affects routing only after a rule references
it.

Domain provider:

```uci
config provider 'youtube_domains'
	option enabled '1'
	option label 'YouTube domains'
	option type 'domain'
	option url 'https://example.com/youtube-domains.txt'
	option auto_update '1'
	option update_hour '3'
	option update_via 'direct'
```

IP provider:

```uci
config provider 'cloudflare_ipv4'
	option enabled '1'
	option label 'Cloudflare IPv4'
	option type 'ip'
	option url 'https://www.cloudflare.com/ips-v4/'
```

Manual update:

On a fresh install, import provider presets from the LuCI Providers page before
using the built-in provider names below.

```sh
zakopd providers update
zakopd providers update cloudflare_ipv4
/etc/init.d/zakop restart
```

Provider cache files live in persistent storage:

```text
/etc/zakop/provider-cache/
```

`/var/lib/zakop/providers/` is a legacy cache path and may disappear on OpenWrt
because `/var` can be linked to volatile `/tmp`.

If a provider cache is missing, compile/startup should warn and continue with
that provider reference empty. Update provider caches manually when needed:

On a fresh install, import provider presets from the LuCI Providers page before
using the built-in provider names below.

```sh
zakopd providers update telegram_ipv4
zakopd compile
```

## Rule Examples

Domain FakeIP proxy rule:

```uci
config rule
	option name 'youtube'
	option enabled '1'
	option priority '100'
	option action 'proxy'
	option outbound 'my_vless'
	option dns_mode 'auto'
	list domain_contains 'youtube'
```

Provider/CIDR packet rule:

```uci
config rule
	option name 'cloudflare_https'
	option enabled '1'
	option priority '200'
	option action 'proxy'
	option outbound 'my_vless'
	option dns_mode 'auto'
	list ip_provider 'cloudflare_ipv4'
	list proto 'tcp'
	list dst_port '443'
```

Mixed rule:

```uci
config rule
	option name 'youtube_and_cloudflare'
	option enabled '1'
	option priority '300'
	option action 'proxy'
	option outbound 'my_vless'
	option dns_mode 'auto'
	list domain_contains 'youtube'
	list ip_provider 'cloudflare_ipv4'
	list proto 'tcp'
	list dst_port '443'
```

Mixed semantics:

- `youtube.com` DNS -> FakeIP, because domain part matched.
- random Cloudflare domain DNS -> real DNS, because provider part is packet-only.
- packet to Cloudflare IP TCP/443 -> proxy through nft.
- port/proto do not affect DNS/FakeIP domain matching.

## DNS Tests

Local zakopd listener:

```sh
dig @127.0.0.1 -p 5353 youtube.com A
dig @127.0.0.1 -p 5353 youtube.com AAAA
dig @127.0.0.1 -p 5353 example.org A
```

sing-box listeners:

```sh
dig @127.0.0.1 -p 15353 youtube.com A
dig @127.0.0.1 -p 15354 example.org A
dig @127.0.0.1 -p 15355 example.org A
```

Expected:

- FakeIP matched `A` returns `198.18.x.x`.
- FakeIP matched `AAAA` returns no real IPv6.
- Non-matching domains use configured real DNS.

From Windows LAN client:

```cmd
ipconfig /flushdns
nslookup -type=A youtube.com 192.168.8.1
nslookup -type=AAAA youtube.com 192.168.8.1
```

If Google DNS returns `Query refused`, check that installed zakopd strips EDNS
Client Subnet:

```sh
zakopd version
logread | grep -E 'zakopd|dnsmasq|sing-box' | tail -n 120
```

## nft/TProxy Checks

Inspect table:

```sh
nft list table inet zakop
```

Expected order:

1. LAN source guard in `prerouting`.
2. non-LAN return.
3. plain DNS return to the later dnsmasq redirect when `manage_dnsmasq=1`.
4. `ct status dnat` return.
5. `direct_clients4` return.
6. `reserved4` return.
7. `proxy_clients4`.
8. FakeIP rule in `custom` mode.
9. ordered provider/CIDR/IP rules.
10. default return.

Check packet port rules:

```sh
nft list table inet zakop | grep -E 'tcp dport|udp dport|sport'
```

Examples:

```text
ip daddr @rule4_0000 tcp dport 443 jump to_proxy_0000
ip daddr @rule4_0000 udp dport 443 jump to_proxy_0000
ip daddr @rule4_0000 tcp dport 1000-2000 jump to_proxy_0000
```

Check TProxy policy routing:

```sh
ip -4 rule show | grep 'fwmark 0x101'
ip -4 route show table 101
```

Expected route:

```text
local default dev lo
```

## DNS Config

DNS terminology:

- `dns_listen`: local zakopd listener used by dnsmasq.
- `singbox_dns_fakeip`: FakeIP listener, default `127.0.0.1:15353`.
- `singbox_dns_real_direct`: real DNS direct listener, default
  `127.0.0.1:15354`.
- `singbox_dns_real_proxy`: real DNS proxy listener, default
  `127.0.0.1:15355`.

Real DNS via DoH:

```uci
option real_dns_mode 'direct'
option real_dns_transport 'https'
option real_dns_server '1.1.1.1:443'
option real_dns_server_name 'cloudflare-dns.com'
option real_dns_path '/dns-query'
```

Real DNS via proxy:

```uci
option real_dns_mode 'proxy'
option real_dns_outbound 'my_vless'
```

`real_dns_mode=proxy` requires a custom outbound. Do not use `proxy_default`.

## dnsmasq Integration

When `manage_dnsmasq=1`, zakop configures dnsmasq to forward DNS to zakopd:

```sh
uci show dhcp.@dnsmasq[0] | grep -E "server|noresolv|addsubnet"
```

Expected:

```text
server='127.0.0.1#5353'
noresolv='1'
addsubnet='32'
```

`addsubnet=32` is used only so zakopd can recover LAN client IP. zakopd strips
EDNS Client Subnet before forwarding DNS to sing-box/public resolvers.

When `manage_dnsmasq=1`, zakop also redirects plain IPv4 LAN DNS on TCP/UDP port
53 to dnsmasq. This is required for domain/FakeIP rules when clients behind a
bridge or AP use another classic DNS server. DoH/DoT bypasses this interception.
Check the rules with:

```sh
nft list chain inet zakop dns_prerouting
```

Expected rules contain `udp dport 53 redirect to :53` and
`tcp dport 53 redirect to :53`.

Stopping zakop should restore previous dnsmasq state:

```sh
/etc/init.d/zakop stop
uci show dhcp.@dnsmasq[0] | grep '127.0.0.1#5353'
```

## Debug Bundle

```sh
zakopd debug
uci show zakop
nft list table inet zakop
grep -nE 'rule_set|rule-set|/tmp/sing-box/rulesets|"detour": "direct"' /tmp/zakop/sing-box.json
logread | grep -E 'zakopd|sing-box|dnsmasq' | tail -n 120
```

`grep` above should not find legacy sing-box rule-set paths or
`"detour": "direct"`.

## Local Archive Test

From repository root:

```sh
GOCACHE=/tmp/zakop-go-cache ./embedded/pack.sh
./scripts/test-archive.sh
./embedded/install.sh --local ./dist/zakop-openwrt-embedded.tar.gz --verbose
```

Dry-run:

```sh
./embedded/install.sh --dry-run
./embedded/install.sh --local ./dist/zakop-openwrt-embedded.tar.gz --dry-run
```

Uninstall on router:

```sh
/usr/share/zakop/uninstall.sh
/usr/share/zakop/uninstall.sh --purge
```

Without `--purge`, `/etc/config/zakop` and `/etc/zakop` are kept for reinstall.
