# Decisions

Этот файл фиксирует project decisions, которые нельзя случайно откатывать.

## D1: zakop Routes Before sing-box

Routing decisions происходят в `nftables` до входа traffic в `sing-box`.

Consequences:

- `direct`/bypass traffic must never enter sing-box;
- sing-box is backend only;
- nft rules are data-plane policy authority.

## D2: sing-box Owns FakeIP

`zakop` не реализует FakeIP allocator в v1. `sing-box` владеет FakeIP и
FakeIP-to-domain mapping.

Consequences:

- FakeIP DNS queries идут в sing-box;
- `zakopd` may forward DNS but must not synthesize FakeIP addresses itself.

## D3: zakopd Is Not a Transparent Proxy

`zakopd` не обрабатывает transparent TCP/UDP data-plane traffic.

Consequences:

- TProxy target is sing-box;
- `zakopd` manages config, DNS forwarding, nft, status;
- `zakopd` must not become proxy engine.

## D4: nftables/firewall4 Only

Поддерживается только `nftables`/firewall4. fw3/iptables support forbidden.

## D5: LAN-Only Capture

`zakop` routes only LAN client traffic. WAN, inbound, router self и non-LAN
prerouting traffic must return before any proxy rule.

Consequences:

- generated nft includes LAN source guard;
- `global` mode is still scoped to LAN sources.

## D6: Embedded-First Distribution

Primary distribution is embedded archive + installer, not `.ipk`.

Installer must auto-detect architecture and never require users to choose binary
manually.

## D7: Managed sing-box Never Overwrites System sing-box

If system `sing-box` is missing or incompatible, managed sing-box lives at:

```text
/usr/libexec/zakop/sing-box
```

Never overwrite:

```text
/usr/bin/sing-box
```

## D8: Provider Is Data, Rule Is Policy

Provider is reusable remote data source. Rule is routing policy.

Consequences:

- creating provider must not create rule;
- creating rule must not create provider;
- IP providers compile into nft sets;
- domain providers load into domain matchers.

## D9: Ordered Rules, First Match Wins

Rules are evaluated by ascending `priority`.

Rule matches when:

- include condition matched;
- exclude condition did not match.

Rules with no include conditions do not match.

## D10: Domain Matchers Are Literal String Operations

Domain matchers are not DNS-aware:

- `domain_equals`: `==`
- `domain_contains`: substring
- `domain_starts_with`: prefix
- `domain_ends_with`: suffix

For root + subdomains:

```uci
list domain_equals 'example.com'
list domain_ends_with '.example.com'
```

## D11: match_all Removed From v1

`match_all` is removed because it duplicates `routing_mode=global` and can
capture too much traffic.

Use:

- `routing_mode=global` to proxy everything globally;
- client `policy=proxy` to proxy one client entirely.

Rules are for explicit domain/IP/provider matches only.

## D12: IPv6 Not Implemented in v1

IPv6 routing is out of scope for v1.

Consequence:

- AAAA for FakeIP domains must not leak real IPv6 while IPv6 routing is absent.

## D13: DNS Transport Belongs To sing-box

`zakopd` is DNS policy layer only. Normal-path DoH/DoT/DoQ clients must not be
implemented in Go inside `zakopd`.

Consequences:

- DNS query enters zakopd on `dns_listen`;
- zakopd selects `fakeip`, `real-direct`, `real-proxy`, or `block`;
- raw DNS wire query is forwarded to local sing-box DNS listener;
- sing-box handles UDP/TCP/DoT/DoH transport.

## D14: Mixed Rules Are Two Entry Points, Not AND

A rule may contain domain matchers and provider/CIDR/IP matchers at the same
time.

Semantics:

- domain matchers are DNS-phase only;
- provider/CIDR/IP matchers are packet/nft-phase only;
- port/proto matchers are packet/nft-phase only;
- mixed rule is not an AND between domain and IP;
- both entry points share one `action` and one `outbound`.
