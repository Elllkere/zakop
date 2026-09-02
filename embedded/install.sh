#!/bin/sh

set -eu

BASE_URL="${ZAKOP_BASE_URL:-https://github.com/elllkere/zakop/releases/latest/download}"
ARCHIVE_NAME="zakop-openwrt-embedded.tar.gz"
WORK_DIR="${TMPDIR:-/tmp}/zakop-install.$$"
MANAGED_SINGBOX="/usr/libexec/zakop/sing-box"
MIN_SINGBOX_VERSION="1.12.0"
LOCAL_ARCHIVE=""
DRY_RUN=0
VERBOSE=0
LANGUAGE_CHOICE=""
EXISTING_INSTALL=0
LEGACY_NETO_INSTALL=0

usage() {
	cat >&2 <<'EOF'
usage: install.sh [--local ./dist/zakop-openwrt-embedded.tar.gz] [--dry-run] [--verbose] [--language en|ru]
EOF
}

while [ "$#" -gt 0 ]; do
	case "$1" in
		--local)
			[ "$#" -ge 2 ] || {
				usage
				exit 1
			}
			LOCAL_ARCHIVE="$2"
			shift 2
			;;
		--dry-run)
			DRY_RUN=1
			shift
			;;
		--verbose)
			VERBOSE=1
			shift
			;;
		--language)
			[ "$#" -ge 2 ] || {
				usage
				exit 1
			}
			case "$2" in
				en|ru)
					LANGUAGE_CHOICE="$2"
					;;
				*)
					usage
					exit 1
					;;
			esac
			shift 2
			;;
		--no-ui-restart)
			# Accepted for compatibility with the v1.4.9 LuCI updater. LuCI
			# resources and ACLs now always require a cache clear and UI restart.
			shift
			;;
		-h|--help)
			usage
			exit 0
			;;
		*)
			usage
			exit 1
			;;
	esac
done

[ "$VERBOSE" -eq 1 ] && set -x

cleanup() {
	rm -rf "$WORK_DIR"
}
trap cleanup EXIT INT TERM

die() {
	echo "zakop install: $*" >&2
	exit 1
}

log() {
	echo "zakop install: $*"
}

dry_log() {
	if [ "$DRY_RUN" -eq 1 ]; then
		log "dry-run: $*"
	fi
}

atomic_install() {
	local src="$1"
	local dest="$2"
	local mode="${3:-0755}"
	local tmp="${dest}.zakop-new.$$"

	rm -f "$tmp"
	if ! cp "$src" "$tmp"; then
		rm -f "$tmp"
		die "failed to stage $dest"
	fi
	if ! chmod "$mode" "$tmp"; then
		rm -f "$tmp"
		die "failed to set permissions on $dest"
	fi
	if ! mv -f "$tmp" "$dest"; then
		rm -f "$tmp"
		die "failed to replace $dest"
	fi
}

need_cmd() {
	command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

version_ge() {
	awk -v a="$1" -v b="$2" 'BEGIN {
		split(a, av, ".");
		split(b, bv, ".");
		for (i = 1; i <= 3; i++) {
			ai = av[i] + 0;
			bi = bv[i] + 0;
			if (ai > bi) exit 0;
			if (ai < bi) exit 1;
		}
		exit 0;
	}'
}

openwrt_version() {
	. /etc/openwrt_release
	echo "${DISTRIB_RELEASE:-0}"
}

distro_id() {
	local id
	id="${DISTRIB_ID:-${ID:-unknown}}"
	echo "$id"
}

is_supported_distro() {
	case "${DISTRIB_ID:-}" in
		OpenWrt|openwrt|ImmortalWrt|immortalwrt) return 0 ;;
	esac
	case "${ID:-}" in
		openwrt|OpenWrt|immortalwrt|ImmortalWrt) return 0 ;;
	esac
	case " ${ID_LIKE:-} " in
		*" openwrt "*|*" lede "*) return 0 ;;
	esac
	return 1
}

detect_pkg_manager() {
	local ver
	ver="$(openwrt_version)"
	if version_ge "$ver" "25.12" && command -v apk >/dev/null 2>&1; then
		echo "apk"
		return
	fi
	if command -v opkg >/dev/null 2>&1; then
		echo "opkg"
		return
	fi
	if command -v apk >/dev/null 2>&1; then
		echo "apk"
		return
	fi
	die "could not find apk or opkg"
}

pkg_update() {
	case "$1" in
		apk) return 0 ;;
		opkg) opkg update ;;
		*) die "unknown package manager: $1" ;;
	esac
}

pkg_install() {
	local pm="$1"
	shift
	case "$pm" in
		apk) apk add "$@" ;;
		opkg) opkg install "$@" ;;
		*) die "unknown package manager: $pm" ;;
	esac
}

collect_arch_hints() {
	{
		uname -m 2>/dev/null || true
		if command -v opkg >/dev/null 2>&1; then
			opkg print-architecture 2>/dev/null | awk '{ print $2 }' || true
		fi
		if [ -r /etc/apk/arch ]; then
			cat /etc/apk/arch
		fi
		if command -v apk >/dev/null 2>&1; then
			apk --print-arch 2>/dev/null || true
		fi
		if [ -r /etc/openwrt_release ]; then
			cat /etc/openwrt_release
		fi
		if command -v ubus >/dev/null 2>&1; then
			ubus call system board 2>/dev/null || true
		fi
	} | tr '[:upper:]' '[:lower:]'
}

detect_arch() {
	if [ -n "${ZAKOP_ARCH:-}" ]; then
		echo "$ZAKOP_ARCH"
		return
	fi

	local hints
	hints="$(collect_arch_hints)"

	case "$hints" in
		*x86_64*|*amd64*) echo "linux-amd64"; return ;;
	esac
	case "$hints" in
		*aarch64*|*arm64*) echo "linux-arm64"; return ;;
	esac
	case "$hints" in
		*armv7l*|*armv7*|*arm_cortex-a*) echo "linux-armv7"; return ;;
	esac
	case "$hints" in
		*mipsel*|*mipsle*) echo "linux-mipsle-softfloat"; return ;;
	esac
	case "$hints" in
		*mips*) echo "linux-mips-softfloat"; return ;;
	esac

	die "unsupported CPU architecture; hints: $(echo "$hints" | tr '\n' ' ')"
}

download() {
	local url="$1"
	local dest="$2"
	local tmp="$dest.tmp"
	local attempts=""

	rm -f "$tmp"
	if curl_usable; then
		attempts="$attempts curl"
		if curl -fsSL --connect-timeout 10 --max-time 300 "$url" -o "$tmp"; then
			mv "$tmp" "$dest"
			return 0
		fi
		rm -f "$tmp"
	elif command -v curl >/dev/null 2>&1; then
		attempts="$attempts broken-curl"
	fi
	if command -v wget >/dev/null 2>&1; then
		attempts="$attempts wget"
		if wget -T 20 -t 2 -O "$tmp" "$url"; then
			mv "$tmp" "$dest"
			return 0
		fi
		rm -f "$tmp"
	fi

	die "failed to download $url; attempted:${attempts:- none}"
}

curl_usable() {
	command -v curl >/dev/null 2>&1 || return 1
	curl --version >/dev/null 2>&1
}

check_runtime_curl() {
	if curl_usable; then
		return 0
	fi
	if command -v curl >/dev/null 2>&1; then
		log "warning: /usr/bin/curl is installed but cannot start"
		log "warning: zakop will install using wget where possible, but provider and subscription updates need a working curl"
	else
		log "warning: curl is not installed; provider and subscription updates need curl"
	fi
}

legacy_neto_detected() {
	local path

	for path in \
		/etc/config/neto \
		/etc/init.d/neto \
		/etc/rc.d/S*neto \
		/etc/rc.d/K*neto \
		/usr/bin/netod \
		/usr/libexec/neto \
		/usr/share/neto \
		/usr/share/luci/menu.d/luci-app-neto.json \
		/usr/share/rpcd/acl.d/luci-app-neto.json \
		/www/luci-static/resources/neto \
		/www/luci-static/resources/neto_* \
		/www/luci-static/resources/view/neto \
		/www/luci-static/resources/view/neto_* \
		/etc/neto \
		/var/lib/neto \
		/tmp/neto \
		/tmp/neto-import.txt \
		/tmp/neto-cron.* \
		/tmp/neto-cron-lines.* \
		/tmp/neto-install.* \
		/tmp/neto-upgrade.* \
		/tmp/neto-upgrade-archive.* \
		/tmp/neto-upgrade-text.* \
		/tmp/neto-archive-test.* \
		/tmp/neto-go-cache \
		/tmp/neto-test \
		/tmp/dnsmasq.d/neto.conf \
		/etc/dnsmasq.d/neto.conf
	do
		[ -e "$path" ] || [ -L "$path" ] || continue
		return 0
	done
	if [ -f /etc/crontabs/root ] && grep -Eq \
		'(^# neto subscriptions (begin|end)$|/usr/bin/netod([[:space:]]|$)|/etc/init\.d/neto([[:space:]]|$)|/usr/share/neto(/|[[:space:]]|$))' \
		/etc/crontabs/root; then
		return 0
	fi
	if command -v pidof >/dev/null 2>&1 && pidof netod >/dev/null 2>&1; then
		return 0
	fi
	if command -v nft >/dev/null 2>&1 && nft list table inet neto >/dev/null 2>&1; then
		return 0
	fi
	return 1
}

remove_legacy_neto_cron() {
	local cron_file="/etc/crontabs/root"
	local tmp="/tmp/zakop-legacy-cron.$$"

	[ -f "$cron_file" ] || return 0
	awk '
		$0 == "# neto subscriptions begin" { skip = 1; next }
		$0 == "# neto subscriptions end" { skip = 0; next }
		!skip && $0 ~ /\/usr\/bin\/netod([[:space:]]|$)/ { next }
		!skip && $0 ~ /\/etc\/init\.d\/neto([[:space:]]|$)/ { next }
		!skip && $0 ~ /\/usr\/share\/neto(\/|[[:space:]]|$)/ { next }
		!skip { print }
	' "$cron_file" >"$tmp"
	cat "$tmp" >"$cron_file"
	rm -f "$tmp"
}

prepare_legacy_neto_migration() {
	local old_managed="/usr/libexec/neto/sing-box"

	[ "$LEGACY_NETO_INSTALL" -eq 1 ] || return 0
	log "legacy neto installation detected; migrating it to zakop"

	if [ -x /etc/init.d/neto ]; then
		/etc/init.d/neto stop >/dev/null 2>&1 || true
		/etc/init.d/neto disable >/dev/null 2>&1 || true
	fi
	if command -v killall >/dev/null 2>&1; then
		killall netod >/dev/null 2>&1 || true
	fi
	if command -v nft >/dev/null 2>&1; then
		nft delete table inet neto >/dev/null 2>&1 || true
	fi
	remove_legacy_neto_cron

	mkdir -p "$WORK_DIR"
	mkdir -p /etc/zakop
	if [ -d /etc/neto ]; then
		cp -R /etc/neto/. /etc/zakop/
	fi
	if [ -d /usr/share/neto ]; then
		mkdir -p /usr/share/zakop
		cp -R /usr/share/neto/. /usr/share/zakop/
	fi
	if [ -d /var/lib/neto/providers ]; then
		mkdir -p /etc/zakop/provider-cache
		cp -R /var/lib/neto/providers/. /etc/zakop/provider-cache/
	fi
	if [ ! -f /etc/config/zakop ] && [ -f /etc/config/neto ]; then
		cp /etc/config/neto /etc/config/zakop
	fi
	if [ -f /etc/config/zakop ]; then
		sed -i \
			-e 's#/usr/libexec/neto/#/usr/libexec/zakop/#g' \
			-e 's#/usr/share/neto/#/usr/share/zakop/#g' \
			-e 's#/etc/neto/#/etc/zakop/#g' \
			-e 's#/var/lib/neto/providers/#/etc/zakop/provider-cache/#g' \
			/etc/config/zakop
	fi

	# Keep a compatible managed binary available as a last-resort source when
	# neither the system package nor the new archive contains sing-box.
	if singbox_compatible "$old_managed"; then
		mkdir -p /usr/libexec/zakop
		atomic_install "$old_managed" "$MANAGED_SINGBOX"
	fi
	log "legacy configuration and provider data prepared for zakop"
}

cleanup_legacy_neto_installation() {
	local path

	[ "$LEGACY_NETO_INSTALL" -eq 1 ] || return 0
	if command -v killall >/dev/null 2>&1; then
		killall netod >/dev/null 2>&1 || true
	fi
	if command -v nft >/dev/null 2>&1; then
		nft delete table inet neto >/dev/null 2>&1 || true
	fi
	remove_legacy_neto_cron
	rm -f /etc/init.d/neto /etc/rc.d/S*neto /etc/rc.d/K*neto /usr/bin/netod
	rm -rf /usr/libexec/neto /usr/share/neto
	rm -f /usr/share/luci/menu.d/luci-app-neto.json
	rm -f /usr/share/rpcd/acl.d/luci-app-neto.json
	for path in \
		/www/luci-static/resources/neto \
		/www/luci-static/resources/neto_* \
		/www/luci-static/resources/view/neto \
		/www/luci-static/resources/view/neto_*
	do
		if [ -e "$path" ] || [ -L "$path" ]; then
			rm -rf "$path"
		fi
	done
	rm -rf /tmp/neto /var/lib/neto /etc/neto
	rm -f /etc/config/neto
	rm -f /tmp/dnsmasq.d/neto.conf /etc/dnsmasq.d/neto.conf
	rm -f \
		/tmp/neto-import.txt \
		/tmp/neto-cron.* \
		/tmp/neto-cron-lines.* \
		/tmp/neto-upgrade.* \
		/tmp/neto-upgrade-archive.* \
		/tmp/neto-upgrade-text.*
	rm -rf \
		/tmp/neto-install.* \
		/tmp/neto-archive-test.* \
		/tmp/neto-go-cache \
		/tmp/neto-test
	if [ -x /etc/init.d/cron ]; then
		/etc/init.d/cron reload >/dev/null 2>&1 || /etc/init.d/cron restart >/dev/null 2>&1 || true
	fi
	if [ -x /etc/init.d/dnsmasq ]; then
		/etc/init.d/dnsmasq reload >/dev/null 2>&1 || /etc/init.d/dnsmasq restart >/dev/null 2>&1 || true
	fi
	log "legacy neto files removed after successful zakop startup"
}

first_version() {
	sed -n 's/.*\([0-9][0-9]*\.[0-9][0-9]*\.[0-9][0-9]*\).*/\1/p' | head -n 1
}

write_singbox_check_config() {
	local path="$1"
	cat >"$path" <<'JSON'
{
  "dns": {
    "servers": [
      { "tag": "local", "type": "udp", "server": "1.1.1.1" },
      { "tag": "fakeip", "type": "fakeip", "inet4_range": "198.18.0.0/15" }
    ],
    "rules": [
      { "query_type": [ "A", "AAAA" ], "server": "fakeip" }
    ],
    "final": "local"
  },
  "inbounds": [
    { "type": "direct", "tag": "dns-in", "listen": "127.0.0.1", "listen_port": 15353 },
    { "type": "tproxy", "tag": "tproxy-in", "listen": "127.0.0.1", "listen_port": 16001 }
  ],
  "outbounds": [
    { "type": "direct", "tag": "direct" },
    { "type": "block", "tag": "blocked" }
  ],
  "route": {
    "rules": [
      { "protocol": "dns", "action": "hijack-dns" }
    ],
    "final": "direct"
  }
}
JSON
}

singbox_compatible() {
	local bin="$1"
	local check_cfg="$WORK_DIR/sing-box-check.json"
	local ver

	[ -x "$bin" ] || return 1
	ver="$("$bin" version 2>/dev/null | first_version || true)"
	[ -n "$ver" ] || return 1
	version_ge "$ver" "$MIN_SINGBOX_VERSION" || return 1

	write_singbox_check_config "$check_cfg"
	"$bin" check -c "$check_cfg" >/dev/null 2>&1
}

set_fresh_singbox_path() {
	local path="$1"
	if command -v uci >/dev/null 2>&1; then
		uci set zakop.main.singbox_bin="$path"
		uci commit zakop
	else
		sed -i "s#option singbox_bin .*#option singbox_bin '$path'#" /etc/config/zakop
	fi
}

netmask_to_prefix() {
	echo "$1" | awk -F. '{
		n = 0;
		for (i = 1; i <= 4; i++) {
			v = $i + 0;
			while (v > 0) {
				n += v % 2;
				v = int(v / 2);
			}
		}
		print n;
	}'
}

network_from_ip_prefix() {
	awk -v ip="$1" -v prefix="$2" '
	BEGIN {
		split(ip, o, ".");
		p = prefix + 0;
		if (p < 0 || p > 32 || ip !~ /^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$/)
			exit 1;
		for (i = 1; i <= 4; i++) {
			if (o[i] < 0 || o[i] > 255)
				exit 1;
			rem = p - ((i - 1) * 8);
			if (rem >= 8)
				n[i] = o[i] + 0;
			else if (rem <= 0)
				n[i] = 0;
			else {
				block = 2 ^ (8 - rem);
				n[i] = int((o[i] + 0) / block) * block;
			}
		}
		printf "%d.%d.%d.%d/%d\n", n[1], n[2], n[3], n[4], p;
	}'
}

detect_lan_subnet() {
	local ipaddr=""
	local netmask=""
	local prefix=""
	local route=""
	local NETWORK=""
	local PREFIX=""
	local addr=""
	local addr_cidr=""
	local normalized=""

	if command -v ip >/dev/null 2>&1; then
		route="$(ip -4 route show dev br-lan scope link 2>/dev/null | awk '{ print $1; exit }' || true)"
		case "$route" in
			*/*)
				echo "$route"
				return
				;;
		esac
	fi

	if command -v uci >/dev/null 2>&1; then
		ipaddr="$(uci -q get network.lan.ipaddr 2>/dev/null || true)"
		netmask="$(uci -q get network.lan.netmask 2>/dev/null || true)"
		case "$ipaddr" in
			*/*)
				addr="${ipaddr%/*}"
				prefix="${ipaddr##*/}"
				normalized="$(network_from_ip_prefix "$addr" "$prefix" 2>/dev/null || true)"
				echo "${normalized:-$ipaddr}"
				return
				;;
		esac
		if [ -n "$ipaddr" ] && [ -n "$netmask" ]; then
			if command -v ipcalc.sh >/dev/null 2>&1; then
				# shellcheck disable=SC2046
				eval $(ipcalc.sh "$ipaddr" "$netmask" 2>/dev/null || true)
				if [ -n "${NETWORK:-}" ] && [ -n "${PREFIX:-}" ]; then
					echo "$NETWORK/$PREFIX"
					return
				fi
			fi
			prefix="$(netmask_to_prefix "$netmask")"
			if [ -n "$prefix" ]; then
				normalized="$(network_from_ip_prefix "$ipaddr" "$prefix" 2>/dev/null || true)"
				echo "${normalized:-$ipaddr/$prefix}"
				return
			fi
		fi
	fi

	if command -v ip >/dev/null 2>&1; then
		addr_cidr="$(ip -4 addr show dev br-lan 2>/dev/null | awk '/ inet / { print $2; exit }')"
		if [ -n "$addr_cidr" ]; then
			addr="${addr_cidr%/*}"
			prefix="${addr_cidr##*/}"
			normalized="$(network_from_ip_prefix "$addr" "$prefix" 2>/dev/null || true)"
			echo "${normalized:-$addr_cidr}"
		fi
		return
	fi

	echo "192.168.8.0/24"
}

choose_language() {
	local ans

	if [ -n "$LANGUAGE_CHOICE" ]; then
		return 0
	fi
	if [ -t 0 ]; then
		printf "zakop install: install Russian LuCI localization? [y/N] "
		read -r ans || ans=""
		case "$ans" in
			y|Y|yes|YES|Yes|д|Д|да|ДА|Да)
				LANGUAGE_CHOICE="ru"
				;;
			*)
				LANGUAGE_CHOICE="en"
				;;
		esac
	else
		LANGUAGE_CHOICE="en"
	fi
}

configure_language() {
	command -v uci >/dev/null 2>&1 || return 0
	[ "$EXISTING_INSTALL" -eq 0 ] || return 0
	case "$LANGUAGE_CHOICE" in
		ru)
			log "enabling Russian LuCI localization"
			uci set zakop.main.language='ru'
			uci set zakop.main.language_ru_installed='1'
			;;
		*)
			uci set zakop.main.language='en'
			uci set zakop.main.language_ru_installed='0'
			;;
	esac
	uci commit zakop
}

fix_luci_permissions() {
	local path

	for path in \
		/www/luci-static/resources/zakop \
		/www/luci-static/resources/zakop_* \
		/www/luci-static/resources/zakop-assets \
		/www/luci-static/resources/view/zakop \
		/www/luci-static/resources/view/zakop_*
	do
		[ -d "$path" ] && chmod 0755 "$path"
	done
	for path in \
		/www/luci-static/resources/zakop/*.js \
		/www/luci-static/resources/zakop_*/*.js \
		/www/luci-static/resources/zakop-assets/* \
		/www/luci-static/resources/view/zakop/*.js \
		/www/luci-static/resources/view/zakop_*/*.js \
		/usr/share/luci/menu.d/luci-app-zakop.json \
		/usr/share/rpcd/acl.d/luci-app-zakop.json
	do
		[ -f "$path" ] && chmod 0644 "$path"
	done
}

ensure_lan_subnet_config() {
	local subnet

	command -v uci >/dev/null 2>&1 || return 0
	if uci -q get zakop.main.lan_subnet >/dev/null 2>&1; then
		return 0
	fi

	subnet="$(detect_lan_subnet)"
	[ -n "$subnet" ] || subnet="192.168.8.0/24"
	log "setting default lan_subnet $subnet"
	uci add_list zakop.main.lan_subnet="$subnet"
	uci commit zakop
}

install_files() {
	local arch="$1"
	local config_created=0
	local path

	[ -x "$WORK_DIR/bin/$arch/zakopd" ] || die "archive does not contain zakopd for $arch"

	mkdir -p /usr/bin /usr/share/zakop /usr/libexec/zakop /etc/config
	atomic_install "$WORK_DIR/bin/$arch/zakopd" /usr/bin/zakopd

	if [ -f /etc/config/zakop ]; then
		rm -f "$WORK_DIR/files/etc/config/zakop"
	else
		config_created=1
	fi

	# Remove only zakop-owned LuCI namespaces from previous releases. The archive
	# installs content-versioned paths so the browser requests fresh module URLs.
	for path in \
		/www/luci-static/resources/zakop \
		/www/luci-static/resources/zakop_* \
		/www/luci-static/resources/zakop-assets \
		/www/luci-static/resources/view/zakop \
		/www/luci-static/resources/view/zakop_*
	do
		[ -d "$path" ] && rm -rf "$path"
	done
	cp -R "$WORK_DIR/files/." /
	fix_luci_permissions
	chmod 0755 /etc/init.d/zakop
	[ -f /usr/share/zakop/run-sing-box-log.sh ] && chmod 0755 /usr/share/zakop/run-sing-box-log.sh
	[ -f /usr/share/zakop/check-version.sh ] && chmod 0755 /usr/share/zakop/check-version.sh
	if [ -d /usr/share/zakop/providers ]; then
		for script in /usr/share/zakop/providers/*.sh; do
			[ -f "$script" ] && chmod 0755 "$script"
		done
	fi

	atomic_install "$WORK_DIR/install.sh" /usr/share/zakop/install.sh
	atomic_install "$WORK_DIR/uninstall.sh" /usr/share/zakop/uninstall.sh
	atomic_install "$WORK_DIR/upgrade.sh" /usr/share/zakop/upgrade.sh

	if singbox_compatible /usr/bin/sing-box; then
		log "using compatible system sing-box"
		if [ "$config_created" -eq 1 ]; then
			set_fresh_singbox_path "/usr/bin/sing-box"
		fi
	elif [ -x "$WORK_DIR/bin/$arch/sing-box" ]; then
		log "installing managed sing-box to $MANAGED_SINGBOX"
		atomic_install "$WORK_DIR/bin/$arch/sing-box" "$MANAGED_SINGBOX"
		if ! singbox_compatible "$MANAGED_SINGBOX"; then
			die "managed sing-box exists but is not compatible"
		fi
		if [ "$config_created" -eq 1 ]; then
			set_fresh_singbox_path "$MANAGED_SINGBOX"
		fi
	elif singbox_compatible "$MANAGED_SINGBOX"; then
		log "using migrated managed sing-box"
	else
		die "no compatible system sing-box and no managed sing-box for $arch in archive"
	fi

	ensure_lan_subnet_config
	configure_language
}

verify_installed_version() {
	local expected="${ZAKOP_EXPECT_VERSION:-}"
	local archive_expected=""
	local actual=""

	if [ -f "$WORK_DIR/zakop-version.txt" ]; then
		archive_expected="$(sed -n '1{s/[[:space:]]//g;p;}' "$WORK_DIR/zakop-version.txt")"
	fi
	if [ -n "$expected" ] && [ -n "$archive_expected" ] && [ "$expected" != "$archive_expected" ]; then
		die "downloaded archive version $archive_expected does not match requested $expected"
	fi
	[ -n "$expected" ] || expected="$archive_expected"
	actual="$(/usr/bin/zakopd version 2>/dev/null | awk '{ print $2; exit }')"
	[ -n "$actual" ] || die "installed zakopd cannot report its version"
	if [ -n "$expected" ] && [ "$actual" != "$expected" ]; then
		die "installed zakopd version $actual does not match expected $expected"
	fi
	log "verified zakopd $actual"
}

zakop_service_pids() {
	command -v ubus >/dev/null 2>&1 || return 0
	ubus call service list '{"name":"zakop"}' 2>/dev/null |
		sed -n 's/^[[:space:]]*"pid":[[:space:]]*\([0-9][0-9]*\),*$/\1/p'
}

zakop_runtime_ready() {
	local status expected

	status="$(/usr/bin/zakopd status 2>/dev/null || true)"
	for expected in \
		"nft_table: present" \
		"ip_rule: present" \
		"local_route: present" \
		"dns_listener: present" \
		"fakeip_dns_listener: present" \
		"real_direct_dns_listener: present" \
		"real_proxy_dns_listener: present" \
		"tproxy_listener: present"
	do
		printf '%s\n' "$status" | grep -Fqx "$expected" || return 1
	done
	return 0
}

restart_zakop_safely() {
	local old_pids="" pid="" alive=0 attempts=0 enabled="1"

	old_pids="$(zakop_service_pids || true)"
	/etc/init.d/zakop stop >/dev/null 2>&1 || true

	if [ -n "$old_pids" ]; then
		while [ "$attempts" -lt 10 ]; do
			alive=0
			for pid in $old_pids; do
				if kill -0 "$pid" 2>/dev/null; then
					alive=1
					break
				fi
			done
			[ "$alive" -eq 0 ] && break
			attempts=$((attempts + 1))
			sleep 1
		done
		[ "$alive" -eq 0 ] || die "old zakop processes did not stop; networking was returned to direct mode"
	else
		# Older procd versions may omit instance PIDs from the service response.
		sleep 2
	fi

	if ! /etc/init.d/zakop start; then
		/etc/init.d/zakop stop >/dev/null 2>&1 || true
		die "service failed to start after update; networking was returned to direct mode"
	fi
	enabled="$(uci -q get zakop.main.enabled 2>/dev/null || true)"
	[ -n "$enabled" ] || enabled="1"
	[ "$enabled" = "1" ] || return 0

	attempts=0
	while [ "$attempts" -lt 15 ]; do
		if /etc/init.d/zakop running >/dev/null 2>&1 && zakop_runtime_ready; then
			log "service restart verified"
			return 0
		fi
		attempts=$((attempts + 1))
		sleep 1
	done

	/etc/init.d/zakop stop >/dev/null 2>&1 || true
	die "service did not become ready after update; networking was returned to direct mode"
}

clear_luci_cache() {
	rm -f /tmp/luci-indexcache /var/run/luci-indexcache
	rm -rf /tmp/luci-modulecache
}

restart_luci_deferred() {
	(
		sleep 2
		if [ -x /etc/init.d/uhttpd ]; then
			/etc/init.d/uhttpd restart || true
		fi
		# rpcd owns the LuCI file.exec process running this installer. Restart it
		# last because procd may terminate this background child together with rpcd.
		if [ -x /etc/init.d/rpcd ]; then
			/etc/init.d/rpcd restart || true
		fi
	) >/dev/null 2>&1 &
}

if [ "$DRY_RUN" -eq 1 ] && [ ! -r /etc/openwrt_release ]; then
	if [ -n "$LOCAL_ARCHIVE" ] && [ ! -f "$LOCAL_ARCHIVE" ]; then
		die "local archive not found: $LOCAL_ARCHIVE"
	fi
	log "dry-run outside OpenWrt/ImmortalWrt; arguments are valid"
	exit 0
fi

[ -r /etc/openwrt_release ] || die "this installer must run on OpenWrt or ImmortalWrt"
. /etc/openwrt_release
if [ -r /etc/os-release ]; then
	. /etc/os-release
fi
is_supported_distro || die "this installer must run on OpenWrt or ImmortalWrt"

ver="$(openwrt_version)"
version_ge "$ver" "23.05" || die "$(distro_id) $ver is unsupported; need 23.05 or newer"

pm="$(detect_pkg_manager)"
arch="$(detect_arch)"
log "detected $(distro_id) $ver, package manager $pm, arch $arch"

if [ -x /usr/bin/zakopd ] && [ -x /etc/init.d/zakop ] && [ -f /etc/config/zakop ]; then
	EXISTING_INSTALL=1
fi
if legacy_neto_detected; then
	LEGACY_NETO_INSTALL=1
	if [ -x /usr/bin/netod ] && [ -x /etc/init.d/neto ] && [ -f /etc/config/neto ]; then
		EXISTING_INSTALL=1
	fi
fi

if [ "$DRY_RUN" -eq 1 ]; then
	dry_log "would install required dependencies with $pm"
	if [ -n "$LOCAL_ARCHIVE" ]; then
		[ -f "$LOCAL_ARCHIVE" ] || die "local archive not found: $LOCAL_ARCHIVE"
		dry_log "would install from local archive $LOCAL_ARCHIVE"
	else
		dry_log "would download $BASE_URL/$ARCHIVE_NAME"
	fi
	if [ -n "$LANGUAGE_CHOICE" ]; then
		dry_log "would configure LuCI language $LANGUAGE_CHOICE"
	else
		dry_log "would ask whether to enable Russian LuCI localization when interactive"
	fi
	dry_log "would install zakopd for $arch and configure dnsmasq/init/LuCI"
	[ "$LEGACY_NETO_INSTALL" -eq 0 ] || dry_log "would migrate the legacy neto config and provider data, then remove old runtime files after startup verification"
	exit 0
fi

if [ "$EXISTING_INSTALL" -eq 0 ]; then
	choose_language
	pkg_update "$pm"
	pkg_install "$pm" \
		luci-base rpcd rpcd-mod-file rpcd-mod-rpcsys ucode \
		nftables-json ip-full ca-bundle curl bind-dig tcpdump \
		kmod-nft-tproxy kmod-nft-socket kmod-nft-nat || die "failed to install required dependencies"

	if ! pkg_install "$pm" sing-box; then
		log "system sing-box package was not installed; managed sing-box will be used if present"
	fi
else
	log "existing installation detected; preserving config and skipping package installation"
fi
check_runtime_curl
prepare_legacy_neto_migration

need_cmd tar
mkdir -p "$WORK_DIR"
if [ -n "$LOCAL_ARCHIVE" ]; then
	[ -f "$LOCAL_ARCHIVE" ] || die "local archive not found: $LOCAL_ARCHIVE"
	cp "$LOCAL_ARCHIVE" "$WORK_DIR/$ARCHIVE_NAME"
else
	download "$BASE_URL/$ARCHIVE_NAME" "$WORK_DIR/$ARCHIVE_NAME"
fi
tar -xzf "$WORK_DIR/$ARCHIVE_NAME" -C "$WORK_DIR"
if [ -d "$WORK_DIR/zakop" ]; then
	WORK_DIR="$WORK_DIR/zakop"
fi

command -v fw4 >/dev/null 2>&1 || die "firewall4/fw4 is required"
command -v nft >/dev/null 2>&1 || die "nftables is required"
command -v ip >/dev/null 2>&1 || die "ip-full is required"

install_files "$arch"
verify_installed_version

/etc/init.d/zakop enable
restart_zakop_safely
if [ "$EXISTING_INSTALL" -eq 1 ]; then
	# The first start replaces the procd instance definitions that were loaded
	# from the previous release. A second clean cycle is intentional: it runs
	# only with the newly installed init script, wrappers, and binaries and
	# matches the manual restart required by older self-updates.
	log "performing final clean service restart after update"
	sleep 2
	restart_zakop_safely
fi
cleanup_legacy_neto_installation

clear_luci_cache
log "installed"
restart_luci_deferred
