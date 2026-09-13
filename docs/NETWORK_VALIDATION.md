# Проверки диагностической сборки

База аудита: `48fce6d`. Release version определяется Git tag;
диагностическая версия имеет суффикс `-network-debug`.
Все выполненные проверки относятся к development host, не к R2S.

| Проверка | Результат / граница |
|---|---|
| `go test ./...` и `go test -tags networkdebug ./...` | PASS; обе сборки |
| Production isolation | PASS: во всех пяти binaries отсутствуют collector path/diagnostic strings; init/wrapper без diagnostic блоков; hotplug/materials отсутствуют |
| RPDB regression | PASS: exact mark/table/full mask; restricted/inverted rules; dev lo prefix |
| Debug nft generation | PASS: production исключает debug, diagnostic включает автоматически; после удаления instrumentation verdicts совпадают |
| Snapshot tests (networkdebug) | PASS: bounded output, secret redaction, permissions, event rotation/cooldown |
| Procd build flavor test | PASS: rendered production без observer, diagnostic с observer; shell stubs, не реальный procd |
| Existing wrapper lifecycle tests | PASS с fake processes: restart after DNS failure и stop child |
| TCP generator smoke | PASS: 3 loopback TCP flows, paced send и полное совпадение echoed bytes; без NAT |
| Shell syntax | PASS для embedded/scripts/init/hotplug |
| ShellCheck | Новые shell scripts PASS. Existing wrapper PASS с исключением прежних SC3043 (`local` в ash) и SC2015 |
| LuCI JavaScript syntax / ACL+menu JSON | PASS; LuCI код не менялся |
| `git diff --check` | PASS |
| Embedded build | Собраны linux-amd64/arm64/armv7/mips-softfloat/mipsle-softfloat |
| `scripts/test-archive.sh` | PASS; проверены в том числе diagnostic artifacts/hotplug |
| nft kernel validation | НЕ ВЫПОЛНЕНА: нужны netlink privileges/ядро с nft TProxy; подготовлена в router check и namespace harness |
| sing-box native config check | НЕ ВЫПОЛНЕНА локально: binary отсутствует; подготовлена в router check/harness |
| Network namespace DNAT integration | НЕ ВЫПОЛНЕН: среда ограничивает netns/netlink; требуется отдельная VM |
| Actual start/stop/restart/reload/idempotency | НЕ ВЫПОЛНЕНЫ на OpenWrt; отдельный router script подготовлен |
| Real RDP и 1–12 h stress | НЕ ВЫПОЛНЕНЫ; тестовые сценарии в checklist |
| NIC watchdog reproduction | НЕ ВЫПОЛНЕНО; causal conclusion отсутствует |

Первый sandbox запуск DNS/loopback tests упёрся в запрет sockets.
Повторный разрешённый локальный запуск DNS и TCP smoke прошёл.
Namespace попытки прекращены после уточнения среды; они не являются проверкой
маршрутизации или драйвера устройства.

## Изменённые файлы

| Файлы | Назначение |
|---|---|
| `internal/tproxy/tproxy.go`, `tproxy_test.go` | Единственный production logic fix: точное распознавание RPDB/route |
| `internal/buildmode/*.go` | Build-tag constants; legacy debug_network option больше не используется |
| `cmd/zakopd/commands_{production,networkdebug}.go` | CLI collector исключён build constraints из production |
| `internal/nft/generator.go`, `debug_test.go`, `scale_test.go` | Observations/counters, verdict equivalence и generator scale |
| `internal/netdebug/snapshot.go`, `snapshot_test.go` | Ограниченный RAM collector, polling, redaction/rotation |
| `cmd/zakopd/main.go` | Команда `network-debug` |
| `embedded/files/etc/init.d/zakop` | Debug-only procd observer |
| `embedded/networkdebug/files/etc/hotplug.d/iface/95-zakop-debug` | Только diagnostic archive: ifdown snapshot |
| `embedded/files/usr/share/zakop/run-sing-box-log.sh` | Exit/health snapshots, сохранение exit code |
| `internal/openwrt/network_debug_test.go` | Исполняемая проверка обоих rendered init scripts со stubs |
| `embedded/install.sh`, `embedded/uninstall.sh` | Удаление owned hotplug при переходе на production/uninstall |
| `embedded/pack.sh`, `scripts/test-archive.sh` | Diagnostic artifacts внутри embedded archive |
| `scripts/dnat-stress.py`, `test-dnat-stress.py` | Endpoint workload и local smoke |
| `scripts/test-dnat-netns.py` | Изолированная DNAT/TProxy интеграционная регрессия |
| `scripts/router-network-check.sh` | Target-only validation и явно выбираемый lifecycle test |
| `docs/NETWORK_AUDIT.md`, `NETWORK_DIAGNOSTICS.md`, этот файл | Отчёт, 11-пунктный checklist, результаты |

Production binary/архив не содержат нового debug кода; diagnostic archive
включает сбор автоматически, независимо от прежнего UCI debug_network.
DNAT bypass не перемещён, already-marked bypass не добавлен, mark/mask policy
не изменена. Существующие lifecycle race risks перечислены в audit и не
объявлены исправленными. Test archive следует сначала проверить на R2S
с локальным доступом; он не является доказанным исправлением watchdog.
