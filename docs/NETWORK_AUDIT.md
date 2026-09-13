# Аудит сетевого пути Zakop

Упаковка диагностики после аудита разделена: обычный архив исключает её
на этапе компиляции/упаковки; `--network-debug` архив включает автоматически.
Старый runtime-флаг `debug_network` больше не используется. Routing findings
ниже от этого разделения не меняются.

## Область доказательств

Исследована рабочая копия на базе `48fce6d`. Источники архитектуры —
`internal/nft/generator.go`, `internal/tproxy/tproxy.go`,
`internal/proxyroute/proxyroute.go`, `internal/singbox/generator.go`,
`internal/dnsproxy/`, `cmd/zakopd/main.go`, init script и wrapper sing-box.
README не использовался как доказательство поведения.

На R2S команды не выполнялись. Нет полного ruleset, RPDB, sysctl, версии
ядра/sing-box, конфигурации zapret и временного ряда с устройства.
Из фактических данных есть предоставленный порядок правил и
`ct status dnat counter packets 71 bytes 10129 return`.
Это подтверждает работу bypass для каких-то DNAT-пакетов; без tuple/trace
нельзя утверждать, что все 71 пакет принадлежат конкретному RDP flow.

Статический анализ **не обнаружил штатного замкнутого packet path**,
который объяснял бы NETDEV WATCHDOG. Он обнаружил риски потери связности,
особенно при apply/reload и сосуществовании с чужими rules. Это разные выводы.
Отсутствие воспроизведения не исключает нагрузочный триггер ошибки NIC/ядра.

Первый отчёт был выдан до изменений. Ниже — расширенный отчёт, включая
последующие уточнения и точное разделение исправлений/неисправленных рисков.

## 1. Current architecture

Zakop генерирует одну таблицу `inet zakop`. Основной ingress hook —
`prerouting`, type filter, priority mangle (-150). В него попадают пакеты,
а не DNS-имена. `from_lan` вызывается при попадании source IPv4 в `lan_subnets4`;
если задан `lan_iface`, дополнительно проверяется `iifname`.

Без `lan_iface` это **проверка source subnet, не доказательство LAN ingress**.
WAN с пересекающимся адресным пространством, VPN и spoofed source могут
удовлетворять тому же условию. Нормальный внешний Internet source — нет.

`zakopd` — DNS forwarder, не прозрачный TCP/UDP proxy. Решение packet phase:
bypass, drop либо TProxy. Sing-box принимает TProxy sockets, восстанавливает
FakeIP domain и открывает собственные outbound соединения.

`proxyroute.Targets()` создаёт target для каждого custom outbound/pool:

| Индекс i | Chain | Inbound | Port |
|---|---|---|---|
| i | `to_proxy_%04d` | `tproxy-%04d-in` | `tproxy_port + i` |

Один outbound, используемый сотней правил, даёт один target. Неиспользуемые
selectable outbounds также получают targets. Несколько inbounds обслуживает
один основной sing-box процесс. Временные процессы latency/update — отдельные,
с localhost mixed inbound/controller и собственными временными конфигурациями.

## 2. Exact packet path

| Traffic | Путь и решение |
|---|---|
| LAN → Internet direct | LAN guard → DNS/DNAT/direct-client/reserved/direct-rule либо unmatched return → RPDB/main → fw4 forward → SNAT/masquerade при необходимости → WAN |
| LAN → proxy | LAN guard → selected jump → mark=0x101 → transparent socket lookup → другие prerouting chains → RPDB → local delivery → fw4 input → sing-box |
| Internet → DNAT | Не совпадает LAN source → fw4 dstnat → LAN route → fw4 forward → LAN host |
| LAN reply DNAT | Conntrack lookup узнаёт reply → `ct status dnat return` → main/WAN → fw4 forward → обратный NAT → Internet |
| Router self | Local output; interception hook Zakop отсутствует. Другие OUTPUT/RPDB rules остаются применимыми |
| Sing-box outbound | Новый локальный socket → output → обычный маршрут к proxy server. В конфигурации нет TUN/auto_route |
| DNS LAN UDP/TCP:53, manage_dnsmasq=1 | mangle DNS return → nat redirect:53 → dnsmasq → zakopd → соответствующий localhost DNS listener sing-box |
| DNS router/non-LAN | `dnsproxy.isLANClient()` исключает loopback/router IP/non-LAN; real DNS, не FakeIP |
| Уже marked ingress | Никакого специального return; обычные правила могут оставить mark либо заменить его при TProxy |
| Established TCP/UDP | Нет общего early established return. Каждый ingress packet проходит packet policy; это нормально для TProxy |
| Related/ICMP | Не превращается в TCP/UDP proxy. DNAT status может обеспечить early return; остальные non-TCP/UDP доходят до return |
| FakeIP | При наличии proxy domain rule FakeIP range направляется в первый target; sing-box reverse mapping + ordered domain rules выбирают outbound |
| Real IP | Ordered rule set lookup + protocol/port match → соответствующий target; direct/block терминальны внутри policy |
| Mixed domain/IP | Domain часть применяется в DNS phase; IP/provider/proto/ports — в packet phase; не логическое AND |

`return` в regular chain возвращает управление вызывающей цепочке.
В данном генераторе после `jump from_lan` идёт `return`, поэтому дальнейших
proxy rules самой Zakop после успешного bypass нет.
`accept` успешного TProxy завершает текущую base chain, но не все base chains
системы. Другие таблицы могут изменить/drop/queue packet далее. [1]

DNS ECS `addsubnet=32` используется как локальная метаинформация клиента;
forwardUDP/forwardTCP вызывают stripClientSubnetOption. FakeIP AAAA получает
локальный пустой ответ при включённом фильтре; это не IPv6 routing.
DoH/DoT клиентов не перехватываются как port 53. `manage_dnsmasq=0` убирает
DNS bypass/redirect: TCP/UDP:53 тогда подчиняется обычной packet policy.

## 3. Exact DNAT/RDP packet path

Для внешнего клиента C, публичного WAN адреса W и Windows H:

```text
C:ephemeral → W:3389
  WAN ingress
  conntrack: NEW, original tuple
  Zakop -150: C не в LAN source → return
  fw4 -100: DNAT W:3389 → H:3389; connection получает IPS_DST_NAT
  route H через LAN → fw4 forward → H

H:3389 → C:ephemeral
  LAN ingress
  conntrack -200: тот же connection, reply direction, IPS_DST_NAT установлен
  Zakop -150: from_lan → ct status dnat → return
  остальные prerouting chains → RPDB/main → WAN
  fw4 forward established/related → postrouting reverse NAT H → W
  W:3389 → C:ephemeral
```

Статус DNAT описывает NAT в ORIGINAL direction и хранится на соединении,
а не только на первом SYN или original skb. В Linux `IPS_DST_NAT` помечен
как неизменяемый после установки. Ответ видит этот статус уже после conntrack
lookup, до reverse NAT в postrouting. [2]

На первом WAN SYN DNAT status ещё может отсутствовать на -150; правильность
этого сценария обеспечивается WAN/LAN guard, а не ожиданием будущего DNAT.
Hairpin из LAN — иной сценарий: первый пакет тоже виден раньше dstnat;
его безопасность зависит от reserved/local destination и ingress config.
Публичный router-owned WAN IP не добавляется динамически в reserved4.
Поэтому вывод о внешнем RDP нельзя автоматически переносить на hairpin.

У внешнего RDP flow без чужих marks таблица 101 не выбирается, sing-box
его не обслуживает. Windows может иметь policy=proxy: DNAT return стоит раньше.
Длительность и двунаправленность TCP не меняют status. Закрытие connection,
conntrack eviction, notrack, разные conntrack zones или асимметрия — уже
другие предпосылки, которые необходимо измерять.

После Zakop возможны вмешательства fw4/custom tables, NFQUEUE и RPDB.
Например, чужой `meta mark set ct mark`, restoring `0x101` после bypass,
даст local route и потерю ответа. Ненулевой bypass counter этого не исключает.
Поздний debug counter на prerouting +300 ловит такой mark до routing,
если вмешательство произошло раньше +300. Он не покрывает любые возможные
hooks/priorities, tc/eBPF и неизвестные таблицы автоматически.

## 4. nft hooks/priorities

Таблица fw4 ниже взята из upstream template, **не из R2S dump**. [3]

| Table/chain | Family/type | Hook | Priority | Назначение |
|---|---|---|---:|---|
| kernel defrag | IPv4 | prerouting | -400 | Reassembly при использовании conntrack |
| fw4/raw_prerouting | inet/filter | prerouting | -300 | notrack/custom raw |
| kernel conntrack | IPv4 | prerouting | -200 | Tuple lookup / connection status |
| zakop/prerouting | inet/filter | prerouting | -150 | LAN packet policy/TProxy |
| fw4/mangle_prerouting | inet/filter | prerouting | -150 | Mark/DSCP/custom mangle |
| zakop/dns_prerouting | inet/nat | prerouting | -100 | LAN DNS redirect, если manage_dnsmasq |
| fw4/dstnat | inet/nat | prerouting | -100 | Port forwards/redirects |
| fw4/prerouting | inet/filter | prerouting | 0 | Helper handling |
| zakop/debug_late_prerouting | inet/filter | prerouting | 300 | Только debug: DNAT + mark observation |
| fw4/mangle_input | inet/filter | input | -150 | Input mangle |
| fw4/input | inet/filter | input | 0 | Local firewall |
| fw4/mangle_forward | inet/filter | forward | -150 | Forward mangle/MSS |
| fw4/forward | inet/filter | forward | 0 | Forward firewall |
| fw4/raw_output | inet/filter | output | -300 | Local notrack |
| fw4/mangle_output | inet/route | output | -150 | Local marking + reroute |
| fw4/output | inet/filter | output | 0 | Local firewall |
| fw4/mangle_postrouting | inet/filter | postrouting | -150 | Egress mangle/MSS |
| fw4/srcnat | inet/nat | postrouting | 100 | SNAT/masquerade |
| kernel conntrack confirm | IPv4 | input/postrouting | last | Confirm tracked flow |

Одинаковый hook/priority между tables не гарантирует стабильного порядка.
У Zakop нет зависимости от fw4 DNAT-before-mangle для внешнего RDP.
Но перекрывающийся LAN DNS DNAT на -100 может конкурировать с redirect Zakop;
mark writer на -150 — с mark set Zakop. Это условный coexistence bug,
не основание передвигать `ct status dnat`.

`nft -o` и `--optimize` в применяющих командах проекта не найдены.
Используются `nft -c -f`, `nft -f`, `nft delete table`. Дополнительное
исследование optimizer в данном проекте не требуется.

## 5–6. Mark 0x101 и RPDB

Генерируется полная запись mark, не OR с отдельным битом:

```text
meta mark set 0x101 tproxy ip to 127.0.0.1:<target-port> accept
ip -4 rule add fwmark 0x101 table 101
ip -4 route add local default dev lo table 101
```

Незаписанная mask у fwmark означает полное сравнение, практически
`0x00000101/0xffffffff`. Установки/сохранения `ct mark` в Zakop нет.
При `0x40000101` exact RPDB rule не совпадёт; нельзя считать mark отдельным
зарезервированным битом. Rule priority явно не задаётся; фактический priority
назначает iproute/kernel, а более ранние RPDB rules могут перехватить lookup.

При обычном RPDB сначала table local, затем подходящая table 101, затем main.
`local default dev lo` — route type LOCAL: ingress skb доставляется локально,
а не посылается по физическому/veth интерфейсу для повторного LAN ingress.
Это стандартная схема transparent proxy. [4]

Нужно проверить на R2S: полную RPDB; все записи 101 (включая более специфичные);
`ip -4 route get <remote> from <LAN-host> iif <LAN-dev> mark 0x101`;
тот же lookup с mark 0; правила других VPN/mwan3/pbr. Сам факт наличия нужной
строки не доказывает, что она первая подходящая и реально используется.

Mark принадлежит skb и не передаётся по Ethernet. Для нового outbound socket
sing-box входящий skb mark не становится автоматически SO_MARK.
При этом системный `tcp_fwmark_accept=1` может влиять на **accepted inbound
TCP socket**, а `fwmark_reflect` — на kernel-generated responses. Это не тот
же outbound socket и не доказательство штатного loop. Эти sysctl теперь
попадают в snapshot; также собираются rp_filter/src_valid_mark/accept_local/
route_localnet для всех interfaces. Нештатные значения требуют отдельного
packet trace и проверки маршрута ответов. [9]

## 7. Sing-box semantics

`Generate()` создаёт TProxy inbound с `listen=127.0.0.1`, без `network`,
поэтому слушаются TCP и UDP. В конфигурации нет routing_mark/default_mark,
TUN, auto_route, route_exclude_address, interface routing и socket mark copy.
Route rules: sniff → DNS hijack → force-client outbounds → proxy domain rules
→ per-inbound target fallback. Final=direct не заменяет targets. [5]

Важное ограничение: domain route rules применяются ко всем TProxy inbounds,
а не только к пакетам, изначально адресованным FakeIP. Sniffed домен real-IP
потока может выбрать иной outbound до per-inbound fallback. Это потенциальное
расхождение с packet-rule outbound при смешанных конфигурациях. Оно не
отправляет direct/bypass flow в sing-box, потому что тот не попал во inbound.
Изменение domain/FakeIP архитектуры в диагностическую сборку не включено.

Установленный TCP transparent socket ищется до нового listener. Это позволяет
обрабатывать следующие пакеты TCP без создания нового proxy connection
на каждый packet. UDP associations живут по правилам sing-box; DNS-only
listeners явно имеют `udp_timeout=10s`, TProxy listeners не задают override.

## 8. Loop / amplification analysis

В штатном графе local delivery заканчивается socket, а outbound начинается
отдельным output. В нём нет обратного ребра к LAN ingress. Поэтому отсутствие
`meta mark 0x101 return` само по себе не является дефектом.
Один и тот же flow может иметь много ingress packets с повторной записью mark:
это ожидаемо и не означает circulation одного skb.

Конкретные дополнительные сценарии:

| Условие | Результат, который действительно следует из условия |
|---|---|
| Нет прозрачного listener | nft TProxy возвращает NFT_BREAK, accept не выполняется, предварительно установленный mark остаётся; управление может вернуться к следующим policy rules |
| Несколько совпавших proxy rules и отсутствующие listeners | Один packet может посетить несколько targets, получить mark несколько раз; обход конечный, bounded числом rules, не бесконечный loop |
| Чужое restore/set mark после DNAT bypass | Local delivery/blackhole вместо WAN; нет доказанного замкнутого пути |
| Чужой OUTPUT mark=0x101 | Outbound может попасть в local route; потенциальная недоступность proxy upstream, не автоматический повторный LAN TProxy |
| Внешний туннель/bridge возвращает egress как LAN ingress | Возможен повторный перехват; требуется конкретная внешняя topology, которой в коде Zakop нет |
| Real DNS upstream настроен обратно на dnsmasq/zakopd | DNS forwarding cycle возможен как конфигурационная ошибка; bounded outstanding forwards/таймауты ограничивают ресурсы, но не делают цикл полезным |
| Перенастройка rules во время established flow | Packet decision может измениться между пакетами; старый proxy socket/session не переносится автоматически |

Семантика отсутствующего listener проверена по `nft_tproxy_eval_v4()` ядра:
на неуспехе устанавливается NFT_BREAK. [6] Это обосновывает новые counters
attempt/success/fallthrough. Existing TProxy counter перед выражением
показывал попытки, а не обязательно успешную передачу в socket.

Нет обнаруженного генератора reject/reset/ICMP flood в Zakop: block rule
использует drop. Возможны обычные TCP retransmits/RST при потере sockets.
Временные update процессы завершаются через deferred Kill/Wait на обычном
выходе; аварийное убийство родителя может обойти defer. Их количество и FD
надо измерять, а не приравнивать каждое появление второго sing-box к утечке.

DNS forwarding ограничен отдельными слотами real/fake и timeout; входящие
TCP connections до чтения query и goroutines для UDP не имеют общего строгого
admission cap. Flood способен дать давление CPU/FD, но длительный RDP TCP
сам по себе не является DNS flood и этого сценария не доказывает.

## 9. Lifecycle и идемпотентность

| Событие | Поведение кода / ограничение |
|---|---|
| Boot/start | check → compile → sing-box check → procd instances → end-to-end real DNS readiness → apply → dnsmasq/cron |
| Repeated apply | Table удаляется, потом создаётся; обычные rules/sets не накапливаются. RPDB PlanEnsure должен пропускать существующие |
| Stop | delete table; цикл удаления current mark/table rules; delete local route; restore dnsmasq; remove own cron |
| Restart | Полный stop/start. Proxy/DNS sessions прерываются. DNAT conntrack не очищается самим Zakop |
| Reload | Используется rc.common/procd start wrapper, отсутствует ошибочный прямой start_service вызов |
| LuCI apply | Helper делает service restart после commit, а не только nft apply |
| WAN/LAN ifdown/ifup, network reload | Triggers для всех logical interfaces кроме loopback; реальный timing зависит от netifd/procd |
| Sing-box crash | Wrapper выходит, procd respawn; nft/RPDB тем временем остаются. Новые proxy packets могут попасть в fallthrough/local blackhole |
| DNS outage | 3 health failures после grace приводят к убийству/restart sing-box, даже если первопричина — WAN/upstream |
| fw4 reload/restart | Обычный upstream работает со своей table. Не эквивалентен полному `flush ruleset`; пользовательские includes могут менять это |
| fw4 flush / чужой flush ruleset | Zakop table исчезает; собственного firewall recovery hook нет. Debug наблюдает отсутствие, но не переустанавливает правила |

`commandApply()` выполняет delete и load разными nft транзакциями, затем
ensureRouting. При первой загрузке существует окно TProxy без готовой RPDB;
при reload — окно отсутствия policy. Ошибка load после delete оставляет таблицу
отсутствующей. Это подтверждённый риск connectivity, не packet loop.

Compile/check/apply используют общие файлы без общей межпроцессной блокировки,
`os.WriteFile` не делает атомарную замену. Два одновременных запуска могут
прочитать/перезаписать разные поколения UCI, nft и JSON. Успех nft -c раньше
load не исключает изменения файла между командами.

Mark/table не журналируются как «ранее применённые». Если UCI изменены,
cleanup использует новые значения, старые entries могут остаться.
PlanCleanup не удаляет чужие routes целиком, что правильно, но ownership
своего прежнего состояния этим не восстанавливается.

Дополнительная находка: procd instances не объявляют `file` config tracking.
При неизменной command line обычный reload может не перезапустить процессы,
хотя UCI/JSON изменились. Проверено по upstream `instance_config_changed()`:
он сравнивает command/env/netdev/file, но не читает произвольный файл из argv.
LuCI restart обходит этот сценарий. [7] Для test build не введён новый порядок
restart/readiness без проверки на реальном procd. В checklist есть тест
изменения конфигурации через reload; пока использовать restart.

Для продолжающегося DNAT/RDP flow исчезновение только Zakop table не убирает
conntrack DNAT state и не добавляет marks; обычный forwarding продолжается.
Разрыв при этом требует иной причины: fw4 policy/NAT, interface/link outage,
чужие marks, conntrack eviction и т. п. Это утверждение условно сохранением
остальных компонентов, а не обещание пережить restart всей сети.

## 10. Реестр проблем

«NIC» ниже означает только гипотезу C: нагрузка может проявить независимую
ошибку драйвера; причинная связь с watchdog ни для одной строки не установлена.

| ID / severity / confidence | File/function; root cause и trigger | Ожидаемые симптомы | Connectivity / CPU / CT growth / loop / NIC | Статус |
|---|---|---|---|---|
| P1 Medium, высокая | `tproxy.RulePresent`: substring match mark/table; чужое 0x1010/table1010 | Пропущенный ensure, ошибочный cleanup | да / нет прямого / нет прямого / нет / не доказано | Исправлено, regression |
| P2 Medium, высокая | `commandApply`: delete/load и nft-before-RPDB | Короткий blackhole/обход proxy, неудачный apply | да / retransmits возможны / не обязательно / не установлен / не доказано | Зафиксировано, instrumentation; transaction fix отложен |
| P3 Medium, высокая по коду | `compile`, init: нет общей сериализации/atomic files | Разные поколения listeners/rules | да / возможен restart churn / возможно / не установлен / не доказано | Нужен lifecycle reproduction |
| P4 Medium, высокая | `stop_service`, cleanupRouting: нет previous mark/table state | Stale entries | да / нет прямого / нет прямого / не установлен / не доказано | Отложено; не менять mark/table в R2S эксперименте |
| P5 Medium, высокая | `lanSourceMatcher`: необязательный iifname | Захват non-LAN с совпавшим source | да / зависит от нагрузки / возможно / только при внешнем return path / не доказано | Явно задать реальный LAN iface в отдельном эксперименте |
| P6 Medium, условная | Закop/fw4 одинаковые priorities при пересечении правил | Порядок DNS redirect/mark зависит от регистрации | да / не обязательно / не обязательно / не установлен / не доказано | Нужен полный ruleset |
| P7 Medium, высокая | Wrapper: DNS outage трактуется как причина restart | Повторные сбросы proxy connections | да / ограниченные probes/restarts / возможно / нет / не доказано | Snapshot dns-health, без изменения health policy |
| P8 Medium, высокая по исходникам | init не задаёт procd `file` | Reload не обновляет живые процессы | да / не обязательно / не обязательно / нет доказанного / не доказано | Проверить actual procd; пока restart |
| P9 Medium, высокая по порядку rules | `domainOutboundRouteRules` до inbound route | Sniffed domain меняет выбранный IP-rule outbound | да/неверный маршрут / нет прямого / нет прямого / нет / не доказано | Отдельный policy issue; DNAT не затрагивает |
| P10 Medium, высокая | TProxy NFT_BREAK при missing socket, mark перед TProxy | Fallthrough, несколько attempts, local blackhole | да / O(matching rules) / не обязательно / конечный обход / не доказано | Новая instrumentation |
| P11 Low–Medium, условная | DNS upstream указывает назад в local DNS path | DNS cycle, slots saturation/SERVFAIL | DNS да / да / возможно / DNS cycle / не доказано | Конфигурационный сценарий, не default |

## What is NOT a bug

- `ct status dnat return` до proxy rules расположен правильно для внешнего DNAT reply.
- Reply DNAT connection сохраняет DNAT status; новое правило обхода не требуется.
- `local default dev lo` не создаёт повторный LAN ingress само по себе.
- Отсутствие `meta mark 0x101 return` без доказанного reentry не является багом.
- Packet mark и conntrack mark — разные сущности; Zakop не использует connmark.
- Успешный target завершает обход Zakop, а не последовательно вызывает все targets.
- Повторная запись mark на разных пакетах TCP flow ожидаема.
- nft interval sets для CIDR — правильная архитектура; каждый CIDR не становится rule.
- Router self не перехватывается OUTPUT chain Zakop: такой chain нет.
- Sing-box logs не пересылаются штатным wrapper в системный log.
- Само число 126 targets не доказывает socket/FD leak.
- NETDEV WATCHDOG — низкоуровневый timeout TX queue; ошибка маршрутизации не равна этому событию.

## Масштаб и измерения

Для 126 targets ожидается 126 TProxy + 3 DNS inbounds, порядка 258 слушающих
TCP/UDP sockets. Controller, активные TCP connections, UDP associations и
temporary update/latency instances добавляют свои FD. Указанный расчёт
нужно сверить с ss и `/proc/<pid>/fd` на R2S.

Локальный amd64 benchmark (Ryzen 7 7800X3D, не R2S; без providers/rules):

| Targets | nft bytes | Generate time | Bytes allocated |
|---:|---:|---:|---:|
| 1 | 1 275 | ~8.1 µs | ~9.5 KB |
| 126 | 17 150 | ~103 µs | ~198 KB |

Это измерение **генератора**, не packet throughput и не kernel memory.
Regression подтверждает 126 target chains, но только два jump в global mode
(force-client и общий global). Target count не делает packet path O(targets).
Selective mode может последовательно проверять O(rules); портовые списки
разворачиваются в Cartesian product source × destination × protocols.
При больших списках ports это отдельный множитель, независимо от CIDR sets.
CPU/softirq/FD/socket/CT slopes нужно измерять на одинаковом traffic rate.

## Zapret/nfqws coexistence

Zakop не управляет zapret и не координирует marks или hook priorities.
Upstream examples zapret используют NFQUEUE postrouting mangle либо после
srcnat; incoming analysis — prerouting; fake packets могут иметь notrack
в раннем OUTPUT. Примеры `DESYNC_MARK=0x40000000` и POSTNAT `0x20000000`
не пересекаются численно с 0x101, но это не универсальное резервирование. [8]

Проверить на R2S все queue/notrack/ct mark/meta mark/SO_MARK/route rules,
конфигурацию выбранной версии и custom scripts. Нельзя выводить actual
priorities установленного zapret из upstream examples. NFQUEUE ACCEPT
продолжает hook traversal; поддельные raw packets nfqws — дополнительный
трафик и могут существенно отличаться от исходного TCP pattern.

Даже исключённый из Zakop RDP может проходить чужой NFQUEUE forward/postrouting.
Proxy outbound sing-box тоже может попадать в zapret как обычный local output.
При типовом ограничении zapret на 80/443 порт3389 обычно вне фильтра, однако
это проверяется по полному ruleset, не по названию сервиса.

## Итог по A / B / C

**A: Zakop не связан.** Совместимо с данными. Независимый st_gmac/RK3328/PHY/
power/IRQ/DMA defect остаётся возможным; смена роли eth0 не исключает его.

**B: Zakop ломает connectivity.** Для ряда config/lifecycle сценариев механизм
найден. Такой сбой может выглядеть как «интернет завис», но не объяснять TX timeout.

**C: Zakop меняет нагрузку и проявляет latent driver bug.** Не доказано и не
опровергнуто. Нужен воспроизводимый workload, timestamps, TX/RX/softirq/CT/FD
и kernel logs, включая фазу до потери сети. Для штатного внешнего RDP
не найден механизм роста числа proxy sockets на каждый его packet.

## Sources

Локальные файлы выше являются первичными источниками фактов о Zakop.
Внешние ссылки проверены для интерпретации Linux/OpenWrt; они не заменяют
версию ядра и полный dump фактического R2S.

1. Netfilter, [Configuring chains](https://wiki.nftables.org/wiki-nftables/index.php/Configuring_chains): base chain traversal/verdicts.
2. Linux, [nf_conntrack_common.h](https://raw.githubusercontent.com/torvalds/linux/master/include/uapi/linux/netfilter/nf_conntrack_common.h): status/IPS_DST_NAT.
3. OpenWrt, [firewall4 ruleset.uc](https://lxr.openwrt.org/source/firewall4/root/usr/share/firewall4/templates/ruleset.uc): hooks; [fw4 entrypoint](https://raw.githubusercontent.com/openwrt/firewall4/master/root/sbin/fw4): reload/flush distinction.
4. Linux, [Transparent proxy support](https://kernel.org/doc/html/latest/networking/tproxy.html): fwmark/local route/socket model.
5. SagerNet, [TProxy inbound](https://sing-box.sagernet.org/configuration/inbound/tproxy/) и [Listen fields](https://sing-box.sagernet.org/configuration/shared/listen/): TCP/UDP listeners, routing_mark.
6. Linux, [nft_tproxy.c](https://raw.githubusercontent.com/torvalds/linux/master/net/netfilter/nft_tproxy.c): established lookup, NFT_BREAK.
7. OpenWrt procd, [instance.c](https://raw.githubusercontent.com/openwrt/procd/master/service/instance.c), `instance_config_changed`/`instance_update`.
8. bol-van, [zapret documentation](https://github.com/bol-van/zapret/blob/master/docs/readme.en.md): NFQUEUE schemes/marks.
9. Linux, [IP sysctl](https://docs.kernel.org/networking/ip-sysctl.html): tcp_fwmark_accept и fwmark_reflect.
