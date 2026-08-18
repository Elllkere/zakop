# Roadmap

Этот файл фиксирует возможные future tasks. Это не product promise.

## Current Status

В текущем состоянии реализовано:

- UCI parsing and validation.
- ordered rules.
- domain include/exclude matchers.
- mixed domain + provider/CIDR/IP rules.
- packet-only `proto`, `src_port`, `dst_port` для IP/provider/CIDR rules.
- DNS policy forwarding through zakopd.
- FakeIP DNS through sing-box.
- real DNS transport through sing-box UDP/TCP/DoT/DoH.
- EDNS Client Subnet stripping before upstream DNS.
- dnsmasq integration.
- nftables generation.
- TProxy policy routing lifecycle.
- native sing-box outbounds: VLESS, Hysteria2, Shadowsocks, Trojan.
- share-link import.
- subscriptions update and auto-update cron.
- remote domain/IP providers.
- built-in Cloudflare/Telegram IP providers.
- provider IPv4 validation/filtering.
- LuCI app for General, Outbounds, Rules, Clients, Providers, Advanced, Debug.
- embedded installer for OpenWrt/ImmortalWrt.

Runtime was tested during development on ImmortalWrt 25.12 rockchip/armv8
aarch64 with `apk`.

## Near-Term Tasks

1. Router-verify LuCI behavior after each UI change.
2. Add more router-side integration tests for generated UCI from LuCI.
3. Improve `zakopd debug` with DNS decision trace examples.
4. Add `zakopd rules list`.
5. Add `zakopd test-domain <client-ip> <domain> <qtype>`.
6. Add better status for provider caches and last update errors.
7. Add clearer LuCI validation messages for packet-level port/proto fields.
8. Add CI release workflow for publishing `zakop-openwrt-embedded.tar.gz`.

## Later Tasks

- More complete provider management.
- More complete subscription/import diagnostics.
- Better generated config diff/debug output.
- More exhaustive sing-box config validation examples.
- Optional advanced DNS settings after current simple DNS model stabilizes.
- IPv6 design, only after v1 IPv4 path is stable.

## Explicitly Not in v1

- IPv6 routing.
- Transparent TCP/UDP proxy inside zakopd.
- Custom FakeIP allocator.
- fw3/iptables support.
- Full `.ipk` packaging.
- Advanced multi-outbound balancing UI.
- Reintroducing `match_all`.
