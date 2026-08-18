# Миграция с neto на zakop

`zakop` использует новые имена для всех компонентов:

| Было | Стало |
|---|---|
| `/etc/config/neto` | `/etc/config/zakop` |
| `/etc/init.d/neto` | `/etc/init.d/zakop` |
| `/usr/bin/netod` | `/usr/bin/zakopd` |
| `/usr/share/neto/` | `/usr/share/zakop/` |
| `/usr/libexec/neto/` | `/usr/libexec/zakop/` |
| `/etc/neto/` | `/etc/zakop/` |
| `/tmp/neto/` | `/tmp/zakop/` |
| nft table `inet neto` | nft table `inet zakop` |
| LuCI `/admin/services/neto` | LuCI `/admin/services/zakop` |
| `NETO_*` | `ZAKOP_*` |

Перед началом убедитесь, что репозиторий GitHub уже переименован в `zakop` и в
последнем Release опубликован asset `zakop-openwrt-embedded.tar.gz`. Не запускайте
старый `neto` и новый `zakop` одновременно: они используют одинаковые DNS- и
TProxy-порты.

## Переименование GitHub-репозитория

До установки на роутер:

1. В GitHub откройте **Settings → General → Repository name**, задайте `zakop`
   и подтвердите переименование.
2. В локальном clone обновите remote:

   ```sh
   git remote set-url origin git@github.com:Elllkere/zakop.git
   git remote -v
   ```

3. При желании переименуйте локальный рабочий каталог из его родительской
   директории: `mv neto zakop`.
4. После merge изменений создайте новый tag/Release. Workflow должен загрузить
   `zakop-openwrt-embedded.tar.gz`, `.sha256` и `zakop-version.txt`.

GitHub обычно сохраняет redirect со старого URL, но installer и self-updater
намеренно используют только новый URL `elllkere/zakop`. Go module path также
изменён на `github.com/elllkere/zakop`.

## Резервная копия

На роутере сохраните конфигурацию и persistent data:

```sh
BACKUP_DIR=/tmp/neto-to-zakop-backup
mkdir -p "$BACKUP_DIR"
cp /etc/config/neto "$BACKUP_DIR/neto.uci"
[ ! -d /etc/neto ] || tar -C /etc -czf "$BACKUP_DIR/etc-neto.tar.gz" neto
[ ! -d /var/lib/neto/providers ] || tar -C /var/lib/neto -czf "$BACKUP_DIR/legacy-providers.tar.gz" providers
```

Скопируйте каталог `/tmp/neto-to-zakop-backup` с роутера на компьютер. `/tmp`
на OpenWrt находится в RAM и исчезнет после перезагрузки.

## Вариант 1: автоматический перенос без сброса настроек

Запустите новый installer поверх работающей установки `neto`:

```sh
sh -c "$(wget -O- https://raw.githubusercontent.com/elllkere/zakop/main/embedded/install.sh)"
```

или:

```sh
sh -c "$(curl -fsSL https://raw.githubusercontent.com/elllkere/zakop/main/embedded/install.sh)"
```

Installer автоматически:

1. обнаружит legacy-установку и остановит `/etc/init.d/neto`;
2. перенесёт UCI-конфигурацию в `/etc/config/zakop`;
3. скопирует `/etc/neto/` в `/etc/zakop/`, старые provider caches из
   `/var/lib/neto/providers/` в `/etc/zakop/provider-cache/`, а пользовательские
   provider scripts из `/usr/share/neto/` — в `/usr/share/zakop/`;
4. перепишет встроенные пути `neto` в перенесённой UCI-конфигурации;
5. установит и запустит `zakop`, затем проверит DNS, nftables, policy routing и
   TProxy listeners;
6. только после успешной проверки удалит старые runtime-файлы, UCI package,
   LuCI namespace, init symlinks, временные install/upgrade/import-файлы, cron
   block `neto` и отдельные старые cron-строки, вызывающие `netod`.

Секции outbounds, pools, clients, rules, providers и subscriptions сохраняют
свои UCI section IDs и порядок. Если запуск `zakop` не пройдёт проверку,
installer завершится до удаления legacy-файлов и оставит сеть в direct mode.

После переноса проверьте:

```sh
uci show zakop
zakopd check
zakopd compile
/etc/init.d/zakop restart
zakopd status
nft list table inet zakop
ip -4 rule show
ip -4 route show table 101
```

В LuCI очистите cache браузера или откройте страницу заново. Installer сам
очищает серверный LuCI cache и перезапускает `uhttpd`/`rpcd`.

### Что проверить вручную

- Внешние скрипты provider должны использовать `ZAKOP_PROVIDER_*` вместо
  `NETO_PROVIDER_*`.
- Cron, hotplug и monitoring scripts должны вызывать `zakopd` и
  `/etc/init.d/zakop`.
- Внешние проверки nftables должны смотреть таблицу `inet zakop`.
- Кастомные абсолютные пути вне `/etc/neto`, `/usr/share/neto`,
  `/usr/libexec/neto` и `/var/lib/neto/providers` installer намеренно не
  изменяет; проверьте их отдельно.
- Если GitHub URL был прописан вручную, замените `elllkere/neto` на
  `elllkere/zakop`, а `NETO_*` override variables на `ZAKOP_*`.

## Вариант 2: полностью удалить и поставить заново

Этот вариант не переносит настройки. Сначала сохраните резервную копию, затем
удалите старую установку вместе с конфигурацией:

```sh
/usr/share/neto/uninstall.sh --purge
```

После этого установите `zakop`:

```sh
sh -c "$(wget -O- https://raw.githubusercontent.com/elllkere/zakop/main/embedded/install.sh)"
```

Для полного сброса уже установленного `zakop` используйте:

```sh
/usr/share/zakop/uninstall.sh --purge
sh -c "$(wget -O- https://raw.githubusercontent.com/elllkere/zakop/main/embedded/install.sh)"
```

Без `--purge` uninstall сохраняет `/etc/config/zakop` и `/etc/zakop`, поэтому
следующая установка восстановит прежнюю конфигурацию вместо чистого старта.

## Вариант 3: ручной перенос из резервной копии

Если `neto` уже удалён, но резервная копия осталась, сначала установите чистый
`zakop`, затем остановите его и замените конфигурацию:

```sh
/etc/init.d/zakop stop
cp /tmp/neto-to-zakop-backup/neto.uci /etc/config/zakop
sed -i \
  -e 's#/usr/libexec/neto/#/usr/libexec/zakop/#g' \
  -e 's#/usr/share/neto/#/usr/share/zakop/#g' \
  -e 's#/etc/neto/#/etc/zakop/#g' \
  -e 's#/var/lib/neto/providers/#/etc/zakop/provider-cache/#g' \
  /etc/config/zakop
```

При наличии архивов восстановите data:

```sh
mkdir -p /etc/zakop/provider-cache
tar -C /tmp -xzf /tmp/neto-to-zakop-backup/etc-neto.tar.gz
cp -R /tmp/neto/. /etc/zakop/
tar -C /tmp -xzf /tmp/neto-to-zakop-backup/legacy-providers.tar.gz
cp -R /tmp/providers/. /etc/zakop/provider-cache/
rm -rf /tmp/neto /tmp/providers
```

Затем проверьте конфигурацию до запуска:

```sh
zakopd check
zakopd compile
/usr/libexec/zakop/sing-box check -c /tmp/zakop/sing-box.json
/etc/init.d/zakop start
zakopd status
```

Если используется system `sing-box`, возьмите фактический путь из
`uci -q get zakop.main.singbox_bin` вместо `/usr/libexec/zakop/sing-box`.
