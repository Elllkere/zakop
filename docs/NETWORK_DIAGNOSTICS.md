# Проверка диагностической сборки на NanoPi R2S

Анализ: [NETWORK_AUDIT.md](NETWORK_AUDIT.md). Эта инструкция не утверждает,
что R2S уже проверен. Runtime проверки nft/sing-box/procd и длительный DNAT
тест ещё должны быть выполнены на тестовом устройстве/подходящей Linux VM.

## 1. Собрать test build

На рабочем Linux ПК, в репозитории:

```sh
GOCACHE=/tmp/zakop-go-cache go test ./...
GOCACHE=/tmp/zakop-go-cache go test -tags networkdebug ./...
GOCACHE=/tmp/zakop-go-cache ./embedded/pack.sh --network-debug
./scripts/test-archive.sh dist/zakop-openwrt-network-debug.tar.gz
```

Архив: `dist/zakop-openwrt-network-debug.tar.gz`. Он содержит каталог `zakop/`,
бинарники пяти архитектур и `diagnostics/` с отчётом и тестами.
Выбор архитектуры при установке делает installer. Для R2S нужен arm64,
но выбирать/подменять бинарник вручную не требуется.

Сохранить на ПК **архив используемой обычной версии** для rollback.
Сохранить отдельную приватную копию `/etc/config/zakop`; она содержит секреты
и не должна попадать в диагностический пакет. Не публиковать её.
Физический/serial доступ особенно полезен для lifecycle tests; выполнять
перезапуски только в согласованное окно, не посреди единственного RDP доступа.

## 2. Установить

Команды копирования выполняются с ПК (подставить адрес router):

```sh
scp dist/zakop-openwrt-network-debug.tar.gz root@ROUTER:/tmp/zakop-debug.tar.gz
```

На R2S:

```sh
mkdir -p /tmp/zakop-test
tar -xzf /tmp/zakop-debug.tar.gz -C /tmp/zakop-test
sh /tmp/zakop-test/zakop/install.sh --local /tmp/zakop-debug.tar.gz
zakopd version
```

Не использовать обычный online installer для test archive: он скачает release.
Installer может устанавливать зависимости при их отсутствии. Он не заменяет
`/usr/bin/sing-box`; версия фактически выбранного sing-box должна быть >=1.12.

## 3. Диагностика включается автоматически

После установки **network-debug архива** сбор уже включён. UCI-флаг не нужен:
старый `debug_network` игнорируется, включая значение `0`. Выбор режима теперь
делается при сборке, а не настройкой на роутере.

Обычная сборка (`./embedded/pack.sh`, без аргументов) создаёт
`dist/zakop-openwrt-embedded.tar.gz` без collector, новой CLI-команды,
инструментации nft, observer/hotplug и диагностических материалов.

В диагностической сборке:

- Counters работают даже при `nft_counters=0`.
- Добавляются observation rules без mark mutation/bypass/log verdicts.
- Добавляется один procd instance `network-debug`, опрос примерно раз в минуту.
- ifdown любого logical interface вызывает event snapshot; loopback/другие
  события тоже могут дать snapshot, но не меняют сеть.
- Wrapper сообщает exit sing-box и DNS-health restart; ручной stop не
  трактуется как crash.
- Наблюдатель отмечает missing nft/RPDB/local route/sing-box и новые выбранные
  kernel events. Он ничего не чинит и не перезапускает.

В этом эксперименте не менять одновременно mark/table, offloads, интерфейсы,
MTU, DNS upstream и zapret strategy. Иначе сравнение теряет смысл.

## 4. Убедиться, что режим работает

```sh
nft list chain inet zakop from_lan
nft list chain inet zakop debug_late_prerouting
nft list counters table inet zakop
zakopd network-debug manual
ls -l /tmp/zakop/network-debug
sh /tmp/zakop-test/zakop/diagnostics/router-network-check.sh check
```

Ищем `debug_already_marked`, `debug_dnat_marked`, late observation и именованные
`debug_tproxy_success`, `debug_tcp_attempt`, `debug_udp_attempt`,
`debug_success_to_proxy_NNNN`. Через минуту появляются `sample-*.txt`.
`router-network-check.sh check` запускает реальную nft/sing-box validation
через `zakopd check` в отдельном временном output directory и DNS readiness.
Он не вызывает apply/reload сети.

Файлы 0600, каталог 0700. Периодические samples ограничены 256 KiB × 12,
event snapshots — 4 MiB × 4; суммарно до ~19 MiB плюс временный сбор.
Автоматические events имеют cooldown 60 секунд; concurrent collection
не запускается. Сбор команды ограничен 3 секундами/2 MiB, полного события —
общим контекстом 45 секунд. Отсутствующие tools/обрезанный вывод отмечаются.
При storm или занятом collector отдельное событие может быть пропущено:
это сознательное ограничение диагностической нагрузки.

Snapshots: date/uptime/memory/load, process names/PIDs без argv, status/FD
sing-box и zakopd, CPU/interrupts/softirq, ip link/addr/rules/routes, nft,
conntrack statistics/count, sockets, ethtool, выбранные сетевые kernel events,
безопасные routing options и relevant sysctl. Вместо raw `ps/top` с возможными
паролями в command line — `/proc` process/status/CPU snapshots. Вместо полного
application `logread`/sing-box.log — фильтр kernel/network events.

Quoted nft strings редактируются; сохраняются фиксированные debug labels и
имена interfaces, существующих в `/sys/class/net`. Дополнительная информация
об interfaces остаётся в `ip link/addr` и config LAN allowlist.
Некоторые произвольные имена/локальные IP сохраняются: пакет приватный,
его всё равно стоит просмотреть перед публикацией. Большие sets могут
обрезаться; summary counters записываются перед full dump.
Не дополнять пакет сырыми UCI, JSON, argv, subscription URLs, ключами.

## 5. Synthetic DNAT/RDP-like test

### На настоящем R2S, endpoints вне роутера

Использовать отдельный TCP port, например 33890. На LAN Linux ПК с Python3:

```sh
python3 dnat-stress.py server --listen LAN_PC_IP --port 33890
```

В LuCI firewall создать **временный** port forward:
WAN TCP/33890 → LAN_PC_IP TCP/33890. Ограничить source IP внешнего тестового
хоста. Не менять существующий RDP forward. Echo server не имеет auth/TLS;
удалить тестовый forward и остановить server после эксперимента.

На внешнем Linux хосте (не на router и не из того же LAN):

```sh
python3 dnat-stress.py client --host PUBLIC_WAN_IP --port 33890 --duration 3600 --mbps 1 --flows 1
python3 dnat-stress.py client --host PUBLIC_WAN_IP --port 33890 --duration 43200 --mbps 1 --flows 4
```

Начать с одного flow/1 Mbps, затем увеличивать только один параметр.
`--mbps` — на **каждый flow в каждом направлении**: 4 flows по 1 Mbps дают
около 4 Mbps туда и 4 Mbps обратно, плюс TCP/IP overhead. Соединения постоянные;
клиент проверяет echoed bytes и **не переподключается**, скрывая разрывы.
На окончании печатает JSON sent/received/errors и возвращает ненулевой код
при повреждении/потере/таймауте. Это TCP transport pattern, не эмулятор RDP codecs.

Optional iperf3: на LAN host `iperf3 -s -p 5201`, временный TCP forward 5201,
снаружи `iperf3 -c PUBLIC_WAN_IP -p 5201 --bidir -P 4 -t 3600 -b 1M`.
Проверить `iperf3 --help` своей версии: `--bidir` доступен не везде.
Основной Python test не требует iperf3.
Синтаксис optional iperf3 подтверждён его [официальной документацией](https://software.es.net/iperf/invoking.html).

### В отдельной Linux VM: автоматическая регрессия

Только VM/test Linux с root/CAP_NET_ADMIN, iproute2, nft, conntrack, sysctl,
Python3 и sing-box. Не запускать на production router для создания namespaces.

```sh
go build -tags networkdebug -o /tmp/zakopd-nettest ./cmd/zakopd
sudo python3 scripts/test-dnat-netns.py --zakopd /tmp/zakopd-nettest --sing-box /PATH/TO/sing-box --duration 60 --flows 4 --reloads 3
```

Harness создаёт Internet → router → LAN namespaces и veth, использует
сгенерированные Zakop nft/JSON и настоящий sing-box. Обычная сеть хоста не
перенастраивается. NAT/filter observation table моделирует relevant hooks,
**но не является запущенным fw4/procd**.

Проверки: positive control LAN flow реально достигает TProxy socket; затем
DNAT workload не увеличивает success counter, marked reply и local-input RDP
counters равны нулю; reply DNAT/WAN counters и реальный Zakop bypass растут;
payload целиком возвращается. В ходе flow выполняется несколько delete/load
Zakop table для воспроизведения текущего apply gap. Counter проверяется перед
reset/reload. Все созданные namespaces/processes удаляются в finally.

Namespace test в рабочей среде агента **не выполнен**: среда не router и
ограничивает netlink/netns, sing-box binary отсутствует. Наличие скрипта —
не успешный integration result.

## 6. Какие counters смотреть

| Counter / rule | Значение |
|---|---|
| `ct status dnat counter … return` | DNAT bypass; для внешнего TCP flow растёт главным образом на ответах LAN |
| `udp/tcp dport 53 counter … return` | DNS bypass, если manage_dnsmasq=1 |
| `ip saddr @direct_clients4` | Hard client bypass |
| `ip daddr @reserved4` | Reserved/local bypass |
| FakeIP range `jump` | Выбор proxy по FakeIP address |
| `ip daddr @rule4_NNNN … jump` | Выбор proxy конкретным packet rule |
| `debug_unmatched_direct` | Остаточный return, не совпавший с предыдущими правилами |
| `debug_already_marked` | Packet уже имел точный Zakop mark при входе в from_lan |
| `debug_dnat_marked` | DNAT flow уже с Zakop mark до bypass |
| `debug_dnat_marked_late` | DNAT flow с Zakop mark на prerouting +300 |
| `debug_tcp_attempt` / `debug_udp_attempt` | Попытки TProxy, включая missing socket |
| `debug_tproxy_success` | Успешное TProxy socket lookup; не доказательство работы remote proxy |
| `debug_success_to_proxy_NNNN` | Успешное socket lookup по target |
| `debug_tproxy_fallthrough` внутри target | TProxy не дошёл до accept; смотреть process/listener/restart/fragment state |

Агрегатные counters всех пользователей нельзя приписывать одному RDP flow.
Для реального теста выбирать тихий период либо narrow trace. Counter reset
при apply/restart нормален: сравнивать дельты внутри одного поколения ruleset.
Зафиксировать wall-clock и uptime, чтобы заметить reboot/NTP jump.

## 7. Обычный real RDP test

1. Записать baseline manual snapshot, время, фактические WAN/LAN dev names,
   kernel/firmware/sing-box versions. Не публиковать URL профилей.
2. Подключиться извне к существующему TCP/3389 forward Windows 11.
3. Сначала обычная работа, затем контролируемое обновление экрана/передача
   данных; сохранить одинаковую длительность для сравнения.
4. Не смешивать с synthetic нагрузкой на первом проходе. Отдельно учитывать,
   использует ли RDP UDP: этот аудит исходно проверяет TCP port-forward.
5. При задержках не делать сразу reload: получить `zakopd network-debug manual`
   через доступный канал, отметить время/направление потери доступности.

Если нужно доказать принадлежность traffic одному flow, временно включить
только узкий trace, максимум на несколько секунд, с локальной консоли:

```sh
nft add table inet zakop_rdp_trace
nft 'add chain inet zakop_rdp_trace pre { type filter hook prerouting priority -151; policy accept; }'
nft add rule inet zakop_rdp_trace pre ip saddr LAN_PC_IP tcp sport 3389 meta nftrace set 1
nft monitor trace
```

Заменить LAN_PC_IP адресом, не копировать placeholder буквально. Остановить
monitor Ctrl-C и обязательно удалить только эту test table:

```sh
nft delete table inet zakop_rdp_trace
```

Trace — отдельная временная инструментальная операция с overhead, по умолчанию
не включена. Полный trace может содержать чужие nft comments; не добавлять
его в публичный snapshot без проверки. Успешный DNAT return с последующим
set mark/queue/TProxy другой таблицей — прямое указание, где искать конфликт.

## 8. Какие файлы забрать и как пережить hard hang

На ПК до reboot:

```sh
scp -r root@ROUTER:/tmp/zakop/network-debug ./r2s-network-debug
```

RAM snapshots **не переживают reboot**. Periodic samples дают предысторию,
пока устройство живо; сами по себе не решают hard hang. Практичный минимум:
удалённый syslog + сбор samples с ПК раз в минуту. У получателя должны быть
rotation/ограничение размера; опрос прекращать по окончании теста.

### Remote syslog

На receiver настроить syslog UDP listener (например, UDP/5514) и rotation.
На R2S, предварительно записав текущие значения этих четырёх options:

```sh
uci set system.@system[0].log_ip='RECEIVER_IP'
uci set system.@system[0].log_port='5514'
uci set system.@system[0].log_proto='udp'
uci set system.@system[0].log_remote='1'
uci commit system
/etc/init.d/log restart
logger -t zakop-debug 'remote syslog test'
```

Это обычная конфигурация logd, не изменение Zakop. Sing-box stdout/stderr
по-прежнему остаются только в `/tmp/zakop/sing-box.log`.
Remote syslog передаёт системные сообщения других служб: хранить приватно,
для обмена отбирать kernel network events. [1]

Если путь к receiver идёт через зависший eth0, финальные сообщения могут
не дойти. Другая доступная физическая сеть/serial console надёжнее ожидания
последнего snapshot после исчезновения LAN.

### Netconsole / pstore

Netconsole отправляет kernel printk через netpoll, но требует поддержки
ядра и драйвера. У USB r8152 netpoll может не поддерживаться; не обещать, что
он заменит eth0 для аварийного вывода. Сначала проверять загрузку модуля и
доставку тестового kernel сообщения; не включать автоматически.
Формат: `netconsole=6665@ROUTER_IP/DEV,6666@RECEIVER_IP/RECEIVER_MAC`.
Все адреса/interface/MAC берутся из реальной конфигурации. При stopped NIC
или полном CPU hang netconsole тоже может молчать. [2]

Проверить существующий `/sys/fs/pstore` и после reboot забрать файлы,
если они появились. Само наличие каталога не означает configured backend.
Ramoops требует заранее зарезервированного RAM/DT region и поддержки ядра;
не добавлять произвольный physical address на R2S. Отсутствие panic/oops
при чистом сетевом hang может не оставить pstore записи. [3]

Не включать постоянный log_file на internal overlay для этого эксперимента.
Если удалённый collector невозможен, редкие записи на отдельный USB storage
— отдельный осознанный вариант; в Zakop автоматические flash writes не добавлены.

## 9. Как обнаружить loop или pathological load

Ненулевой already-marked **не доказывает loop**: другая служба могла выставить
тот же mark впервые. Нужны tuple/trace/interface correlation и одинаковые
пакеты на повторных проходах, отношение счётчиков к ingress, а не один total.

- Для DNAT теста success/attempt TProxy должен оставаться без дельты именно
  этого flow. Late marked DNAT должен быть 0 без чужой маркировки.
- Рост attempts с flat success и fallthrough означает missing socket либо
  неуспех TProxy, не замкнутый маршрут.
- Рост повторных packets вместе с TCP retransmissions и отсутствием route
  означает возможный blackhole. Проверить RPDB route get и late marks.
- Packet circulation подтверждать trace/tcpdump на обоих interfaces с тем же
  tuple/sequence; TTL/повторный ingress/OUTPUT path помогают локализовать петлю.
- Для ресурсоёмкости сравнить CPU, NET_RX/NET_TX softirq, IRQ, drops, CT count,
  socket/FD count в минутных samples при постоянных flows и bandwidth.
- Линейный рост FD/CT после warm-up, не уходящий после окончания flow и
  соответствующих timeout, подозрителен; большой стабильный baseline не leak.

## 10. Как проверить гипотезу «Zakop не связан»

Провести одинаковые окна workload с неизменными offloads/MTU/interface roles:

| Окно | Zakop | Zapret | Назначение |
|---|---|---|---|
| A | Обычная версия | Как обычно | Контроль без debug overhead |
| B | Network-debug archive | Как обычно | Наблюдение DNAT path |
| C | Stopped | Как обычно | Исключение Zakop nft/RPDB/sockets |
| D | Как B | Stopped | Отдельная проверка coexistence |

Остановку выполнять на тестовом router с локальным доступом. После stop
проверить отсутствие table zakop и rule/route именно текущего mark/table;
при ранее менявшихся значениях возможны старые entries — не удалять все
ip rules/route tables. DNAT/firewall forwarding должны оставаться настроены.

Watchdog в окне C при подтверждённом отсутствии Zakop state усиливает A.
Успешный длительный B при чистом DNAT path ослабляет версию прямой ошибки
RDP interception, но не доказывает невозможность редкого NIC bug.
Окно без происшествия меньше типичного интервала отказов малоинформативно.
Сообщать отдельно: connectivity failure, CPU/CT pressure, kernel watchdog.

### Lifecycle tests

На R2S с локальной консоли, отдельно от исходного baseline:

```sh
sh /tmp/zakop-test/zakop/diagnostics/router-network-check.sh lifecycle
```

Script делает stop/stop/start/start/reload/restart два раза, проверяет
отсутствие table после stop, валидность generated config, DNS readiness и
неизменность полного RPDB после repeated start/reload. Он **не** обещает
отсутствие stale entries от старых mark/table и не проверяет procd внутри VM.
PID/listener counts сравнить до/после, включая temporary update processes.

Во время отдельного synthetic flow выполнить `/etc/init.d/firewall reload`:
DNAT counters/connection должны сохраняться при неизменном fw4 configuration.
Не подменять это `fw4 flush`. LAN/WAN ifdown/ifup намеренно рвёт физический
path; проверять восстановление и появление event snapshot после ifup, а не
обещать непрерывный TCP в отсутствие линка.

Изменение outbound port/конфигурации через **reload** дополнительно сравнить
с actual listening sockets и PID: procd config file tracking сейчас отсутствует.
Это известная незакрытая находка; обычный LuCI apply делает restart.
Не менять реальные credentials ради проверки; использовать отдельный тестовый
outbound и вернуть его после проверки.

## 11. Полностью откатиться

Установить обычный `zakop-openwrt-embedded.tar.gz` тем же способом через его
installer с `--local`. Новый installer удаляет оставшийся diagnostic hotplug
hook, заменяет init/wrapper и перезапускает сервис без observer. UCI менять
не требуется; stale `debug_network=1` не может включить отсутствующий код.

При возврате на старую release-версию, выпущенную до поддержки двух архивов,
дополнительно удалить ровно `/etc/hotplug.d/iface/95-zakop-debug`: старый
installer ещё не знает об этом файле. При необходимости вернуть приватную
копию UCI и выполнить restart.

Удалить временный DNAT forward/trace table, остановить stress endpoints,
вернуть прежние logd options, если они менялись. Собранные RAM snapshots не
мешают production и исчезнут после reboot. Автоматического reboot нет.

## Changes / risk assessment

Production change: точное числовое сравнение fwmark, полной mask и table в
RPDB inspection; исключение restricted/inverted rules и совпадения префикса
имени `lo`. Существующие нормальные `0x101 → table101` команды не меняются.

Debug-only (build tag `networkdebug`): nft observation/success counters, late observation hook,
bounded RAM snapshot command, procd observer, ifdown/exit/health event calls.
Default routing verdicts сохраняются; тест сравнивает сгенерированные rules
после удаления instrumentation. Нагрузка debug ненулевая: counters per packet,
раз в минуту несколько diagnostic commands, до ~19 MiB RAM files.

Test-only: Python TCP workload, namespace harness, router validation script,
отчёт и checklist в архиве. Никакие stress tests не запускаются installer.

Не изменены: DNAT bypass, mark value/mask policy, TProxy ports architecture,
apply atomicity, lifecycle restart semantics, запret, драйвер/MTU/offloads.
Незакрытые риски подробно перечислены в audit; сборка не является заявлением,
что watchdog исправлен или все lifecycle races устранены.

## Sources

1. OpenWrt, [Logging messages](https://openwrt.org/docs/guide-user/base-system/log.essentials) и [system configuration](https://openwrt.org/docs/guide-user/base-system/system_configuration).
2. Linux, [Netconsole](https://docs.kernel.org/networking/netconsole.html).
3. Linux, [Ramoops](https://cdn.kernel.org/doc/html/latest/admin-guide/ramoops.html).
