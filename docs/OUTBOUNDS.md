# Outbounds

`outbound` - это sing-box proxy profile, который выбирается в `rule` с
`action=proxy` или в client policy с `policy=proxy`.

Термины `direct`, `blocked`, `proxy`, `outbound`, `import`, `subscription`
оставлены как config/UI names.

## Built-ins

Built-in outbound tags:

- `direct`
- `blocked`

Они всегда генерируются для sing-box и не должны создаваться как
`config outbound`.

`proxy_default` deprecated. LuCI не должен создавать или предлагать
`proxy_default`. Старые rules с `option outbound 'proxy_default'` могут быть
normalized в первый custom outbound для compatibility.

## Creatable Outbound Types

LuCI Outbounds page создает только real proxy profiles:

- `vless`
- `hysteria2`
- `shadowsocks`
- `trojan`

Outbounds table остается компактной:

- label/name;
- type;
- address/server;
- port.

Protocol-specific fields находятся в edit dialog.

## Latency Test

LuCI Outbounds page exposes `Test latency`. The test starts temporary sing-box
with a Clash API controller bound to localhost. It calls sing-box's native
`GET /proxies/{tag}/delay` URLTest endpoint for every real outbound and writes
the result into the read-only `URLTest delay` column of the Outbounds table.
The fastest successful result is highlighted; failed nodes show the error in
the cell tooltip. LuCI sends one named test request at a time and updates the
table incrementally, so a large subscription cannot exceed the XHR timeout as
one monolithic request. A timeout or failure affects only that outbound and
does not stop the remaining tests.

The all-outbounds CLI command processes large subscriptions in batches of 32,
with four requests in parallel inside each batch. The temporary test does not
add nftables rules and does not route router self traffic through neto TProxy.

The same report is available as JSON from the CLI:

```sh
netod outbounds latency
netod outbounds latency my_outbound
```

The first successful URLTest pass warms DNS and protocol state and is discarded;
the second pass is reported. The value is sing-box URLTest delay through the
proxy, not curl process startup time and not an ICMP echo to the server address.
This also verifies that proxy authentication and transport settings work. The
test does not create an `auto`/`urltest` routing outbound or change rule routing.

## Priority Pools

`config outbound_pool` groups at least two concrete outbounds in strict priority
order. Pools are managed on the Outbounds page and appear anywhere neto offers
an outbound selector: rules, proxy clients, simple mode, proxy DNS, provider and
subscription updates, and the neto updater.

```uci
config outbound_pool 'main_pool'
	option label 'Main failover'
	list outbound 'primary'
	list outbound 'backup'
	option check_url 'https://www.gstatic.com/generate_204'
	option check_interval '60'
```

The list order is the priority order. netod checks `primary` first and stops at
the first successful sing-box delay probe. It selects that member through the
localhost-only sing-box Clash API. If `primary` recovers, the next check returns
the pool to it. This is deterministic failover, not URLTest lowest-latency load
balancing. A selection change interrupts existing connections so new
connections immediately use the selected member.

Pool constraints:

- tags are unique across concrete outbounds and pools;
- members are unique concrete outbounds; nested pools are not supported;
- a pool needs at least two members;
- `check_interval` is 5-86400 seconds;
- `check_url` must be an HTTP or HTTPS URL;
- the management controller defaults to `127.0.0.1:19090` and never adds
  router-self nft/TProxy routing.

If every member is unavailable, netod keeps the last selector value and retries
with a short bounded backoff until one member works. Temporary proxy operations
(subscription/provider/self updates) do not use the running selector: they try
the pool members directly and sequentially in priority order.

## Rule Selection

Rule with `action=proxy` выбирает custom outbound или priority pool:

```uci
config rule
	option action 'proxy'
	option outbound 'my_vless'
```

`direct` и `blocked` относятся к action/built-in behavior, а не к proxy outbound
selection в Rules UI.

`Auto` и пустой `Select outbound` не используются. Новый proxy rule сразу
получает первый custom outbound в порядке UCI. Если custom outbounds отсутствуют,
LuCI не создает proxy rule, а backend validation отклоняет такой config.

Выбор применяется независимо для каждого rule:

- IP/CIDR/provider packet matches направляются nftables на TProxy inbound
  выбранного outbound;
- FakeIP domain matches восстанавливают домен через sing-box FakeIP mapping и
  затем попадают в ordered sing-box domain route rule с тем же outbound.

## Client Selection

Client with `policy=proxy` may select a custom outbound:

```uci
config client
	option ip '192.168.8.100'
	option policy 'proxy'
	option outbound 'my_vless'
```

If a legacy proxy client has no `outbound`, config loading selects the first
custom outbound. LuCI does not expose an empty or `Auto` selection.

## Imports

`netod import-uri` импортирует share links и создает обычные `config outbound`
sections.

Supported schemes:

- `vless://`
- `hysteria2://`
- `hy2://`
- `ss://`
- `trojan://`

Manual import:

```sh
cat >/tmp/neto-import.txt <<'EOF'
vless://UUID@example.com:443?security=reality&sni=example.com&pbk=PUBLIC_KEY&sid=SHORT_ID#My%20VLESS
EOF

netod import-uri -file /tmp/neto-import.txt
/etc/init.d/neto restart
```

Imported nodes:

- carry `option imported '1'`;
- are visible in Outbounds table;
- are selectable in Rules outbound dropdown;
- are ordinary editable outbounds.

## Subscriptions

Subscription config:

```uci
config subscription 'my_sub'
	option enabled '1'
	option label 'My subscription'
	option url 'https://example.com/subscription'
	option auto_update '0'
	option update_schedule 'time'
	option update_hour '0'
	option update_via 'direct'
```

Manual update:

```sh
netod subscriptions update my_sub
/etc/init.d/neto restart
```

Omit the name in the CLI to update every enabled subscription:

```sh
netod subscriptions update
```

The LuCI subscriptions section exposes `Update all`, but intentionally sends
one named update request at a time to stay below the LuCI XHR timeout. It shows
progress, continues after individual failures, and restarts neto once after the
full sequence.

Subscription nodes are ordinary outbound sections:

```uci
config outbound 'my_sub_ab12cd34ef'
	option imported '1'
	option subscription 'my_sub'
	option tag 'my_sub_ab12cd34ef'
	option label 'Imported node'
	option type 'vless'
```

Updating a subscription replaces only nodes with matching
`option subscription`.

## update_via proxy

`update_via 'proxy'` does not change nft routing and does not capture
router-self traffic. It starts temporary sing-box local mixed proxy and uses
`curl` through selected outbound.

```uci
config subscription 'my_sub'
	option enabled '1'
	option url 'https://example.com/subscription'
	option auto_update '1'
	option update_schedule 'interval'
	option update_interval_minutes '360'
	option update_via 'proxy'
	option update_outbound 'my_vless'
```

Provider updates use the same direct/proxy update model.

## VLESS + REALITY

```uci
config outbound 'my_vless'
	option tag 'my_vless'
	option label 'My VLESS'
	option type 'vless'
	option server 'example.com'
	option port '443'
	option uuid 'a3482e88-686a-4a58-8126-99c9df64b060'
	option flow 'xtls-rprx-vision'
	option tls '1'
	option server_name 'www.example.com'
	option reality '1'
	option reality_public_key 'PUBLIC_KEY'
	option reality_short_id '0123456789abcdef'
	option alpn 'h2,http/1.1'
	option tls_min_version '1.2'
	option tls_max_version '1.3'
	option utls_fingerprint 'chrome'
	option transport 'tcp'
	option packet_encoding 'xudp'
```

## Hysteria2

```uci
config outbound 'my_hy2'
	option tag 'my_hy2'
	option label 'My Hysteria2'
	option type 'hysteria2'
	option server 'example.com'
	option port '443'
	option password 'PASSWORD'
	option server_name 'example.com'
	option insecure '0'
	option hysteria_obfs_type 'salamander'
	option hysteria_obfs_password 'OBFS_PASSWORD'
	option hysteria_down_mbps '100'
	option hysteria_up_mbps '20'
```

## Shadowsocks

```uci
config outbound 'my_ss'
	option tag 'my_ss'
	option label 'My Shadowsocks'
	option type 'shadowsocks'
	option server 'example.com'
	option port '8388'
	option method '2022-blake3-aes-128-gcm'
	option password 'PASSWORD'
```

## Trojan TLS

```uci
config outbound 'my_trojan'
	option tag 'my_trojan'
	option label 'My Trojan'
	option type 'trojan'
	option server 'example.com'
	option port '443'
	option password 'PASSWORD'
	option tls '1'
	option server_name 'example.com'
	option insecure '0'
	option tls_min_version '1.2'
	option tls_max_version '1.3'
	option transport 'ws'
	option ws_host 'example.com'
	option ws_path '/ws'
	option websocket_early_data '2048'
	option websocket_early_data_header 'Sec-WebSocket-Protocol'
```
