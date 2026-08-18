# Testing

Документ описывает local checks и router checks для `zakop`.

## Local Tests

Запускать из repository root:

```sh
go test ./...
sh -n embedded/*.sh scripts/*.sh
jq empty embedded/files/usr/share/luci/menu.d/*.json
jq empty embedded/files/usr/share/rpcd/acl.d/*.json
node --check embedded/files/www/luci-static/resources/view/zakop/*.js
./embedded/pack.sh
./scripts/test-archive.sh
```

Если sandbox не может писать в default Go cache:

```sh
GOCACHE=/tmp/zakop-go-cache go test ./...
GOCACHE=/tmp/zakop-go-cache ./embedded/pack.sh
```

## Archive Checks

Embedded archive:

```text
dist/zakop-openwrt-embedded.tar.gz
```

Проверить layout:

```sh
tar -tzf dist/zakop-openwrt-embedded.tar.gz | sed -n '1,40p'
```

Expected first line:

```text
zakop/
```

Automated archive check:

```sh
./scripts/test-archive.sh
```

## Router Smoke Test

На OpenWrt/ImmortalWrt router:

```sh
/etc/init.d/zakop restart
zakopd check
zakopd compile
zakopd status
zakopd debug
nft list table inet zakop
ip -4 rule show
ip -4 route show table 101
```

Validate sing-box config:

```sh
/usr/libexec/zakop/sing-box check -c /tmp/zakop/sing-box.json
```

or, if system sing-box is used:

```sh
sing-box check -c /tmp/zakop/sing-box.json
```

## DNS Tests

Local zakopd listener:

```sh
dig @127.0.0.1 -p 5353 youtube.com A
dig @127.0.0.1 -p 5353 youtube.com AAAA
dig @127.0.0.1 -p 5353 example.org A
```

sing-box DNS listeners:

```sh
dig @127.0.0.1 -p 15353 youtube.com A
dig @127.0.0.1 -p 15354 example.org A
dig @127.0.0.1 -p 15355 example.org A
```

Expected:

- FakeIP matched `A` returns `198.18.x.x`.
- FakeIP matched `AAAA` does not return real IPv6.
- Non-matching domains use real DNS.
- Direct clients get real DNS only.

## Windows LAN Client Tests

```cmd
ipconfig /flushdns
nslookup -type=A youtube.com 192.168.8.1
nslookup -type=AAAA youtube.com 192.168.8.1
curl -4 -v --connect-timeout 10 https://example.org
curl -4 -v --connect-timeout 10 https://youtube.com
```

Expected:

- direct clients receive real DNS answers only;
- FakeIP domains return FakeIP for `A`;
- FakeIP domains do not leak real IPv6 via `AAAA`;
- non-LAN/WAN/inbound/router-self traffic is not captured.

## nft/TProxy Checks

Check rule order:

```sh
nft list table inet zakop
```

Expected order:

1. LAN source guard in `prerouting`.
2. non-LAN return.
3. `ct status dnat` return.
4. `direct_clients4` return.
5. `reserved4` return.
6. `proxy_clients4`.
7. FakeIP/provider/rule proxy rules.
8. default return.

Check TProxy route:

```sh
ip -4 rule show | grep 'fwmark 0x101'
ip -4 route show table 101
```

Expected route:

```text
local default dev lo
```

On OpenWrt/ImmortalWrt, `ip -4 route show table 101` may exit with code 2 when
table does not exist. That means missing state, not fatal command failure.

## Provider Checks

First use the LuCI Providers page `Import provider presets` action when checking
built-in provider names on a fresh install.

```sh
zakopd providers update
zakopd providers update cloudflare_ipv4
zakopd providers update telegram_ipv4
ls -la /etc/zakop/provider-cache/
```

Telegram provider must save only IPv4 entries. IPv6 lines from the Telegram feed
are ignored.

## Import/Subscription Checks

Manual import:

```sh
zakopd import-uri -file /tmp/zakop-import.txt
uci show zakop | grep '=outbound'
```

Subscription update:

```sh
zakopd subscriptions update my_sub
uci show zakop | grep "subscription='my_sub'"
```

Cron block:

```sh
grep -n "zakop subscriptions" /etc/crontabs/root
grep -n "zakopd providers update" /etc/crontabs/root
```

## LuCI Manual Checks

Verify on actual OpenWrt/ImmortalWrt LuCI:

- General shows zakop/sing-box status and versions.
- General Start/Stop calls `/etc/init.d/zakop start|stop`.
- General Autostart calls `/etc/init.d/zakop enable|disable`.
- General exposes DNS preset/mode/outbound/transport and routing mode.
- Advanced contains DNS listener, FakeIP range, AAAA filter, dnsmasq, LAN,
  sing-box listener, TProxy and nft settings.
- Rules page writes explicit `enabled` and `priority`.
- Moving rules rewrites priority as `100`, `200`, `300`, ...
- Rules page writes only current matcher fields, not deprecated aliases.
- Rules page hides `dns_mode` and writes `dns_mode=auto`.
- Domain input supports fields, textbox and domain providers.
- IP input supports inline IP/CIDR list, textbox and IP providers.
- Rules details include packet-only Protocol, Source ports and Destination
  ports.
- Ports are not shown in main rules table.
- Rules page does not create providers.
- Providers page does not create rules.
- Outbounds page creates only VLESS, Hysteria2, Shadowsocks or Trojan profiles.
- Outbounds page does not create `proxy_default`.
- Imported/subscription nodes appear in Outbounds and in Rules outbound
  dropdown.

Inspect UCI after LuCI save:

```sh
uci show zakop
```

## Debug Commands

```sh
zakopd debug
logread | grep -E 'zakopd|sing-box|dnsmasq' | tail -n 120
grep -nE 'rule_set|rule-set|/tmp/sing-box/rulesets|"detour": "direct"' /tmp/zakop/sing-box.json
```

The grep check should not find legacy sing-box rule-set paths or
`"detour": "direct"`.
