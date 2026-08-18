#!/bin/sh

set -eu

INSTALL_URL="${ZAKOP_INSTALL_URL:-https://raw.githubusercontent.com/elllkere/zakop/main/embedded/install.sh}"
VERSION_URL="${ZAKOP_VERSION_URL:-https://github.com/elllkere/zakop/releases/latest/download/zakop-version.txt}"
RELEASE_API_URL="${ZAKOP_RELEASE_API_URL:-https://api.github.com/repos/elllkere/zakop/releases/latest}"
ARCHIVE_URL="${ZAKOP_ARCHIVE_URL:-https://github.com/elllkere/zakop/releases/latest/download/zakop-openwrt-embedded.tar.gz}"
ZAKOPD_BIN="${ZAKOP_ZAKOPD_BIN:-/usr/bin/zakopd}"
TMP="${TMPDIR:-/tmp}/zakop-upgrade.$$"
ARCHIVE_TMP="${TMPDIR:-/tmp}/zakop-upgrade-archive.$$"
TEXT_TMP="${TMPDIR:-/tmp}/zakop-upgrade-text.$$"
UPGRADE_LOG="${ZAKOP_UPGRADE_LOG:-/tmp/zakop/upgrade.log}"
MODE="upgrade"
UPDATE_VIA="${ZAKOP_UPDATE_VIA:-}"
UPDATE_OUTBOUND="${ZAKOP_UPDATE_OUTBOUND:-}"

usage() {
	echo "usage: upgrade.sh [--check|--luci]" >&2
}

case "${1:-}" in
	"") ;;
	--check) MODE="check" ;;
	--luci) MODE="luci" ;;
	-h|--help)
		usage
		exit 0
		;;
	*)
		usage
		exit 1
		;;
esac

cleanup() {
	rm -f "$TMP" "$TMP.tmp" "$ARCHIVE_TMP" "$ARCHIVE_TMP.tmp" "$TEXT_TMP" "$TEXT_TMP.tmp"
}
trap cleanup EXIT INT TERM

if [ -z "$UPDATE_VIA" ] && command -v uci >/dev/null 2>&1; then
	UPDATE_VIA="$(uci -q get zakop.main.update_via 2>/dev/null || true)"
fi
[ -n "$UPDATE_VIA" ] || UPDATE_VIA="direct"
if [ -z "$UPDATE_OUTBOUND" ] && command -v uci >/dev/null 2>&1; then
	UPDATE_OUTBOUND="$(uci -q get zakop.main.update_outbound 2>/dev/null || true)"
fi
case "$UPDATE_VIA" in
	direct|proxy) ;;
	*)
		echo "zakop upgrade: unsupported update_via $UPDATE_VIA" >&2
		exit 1
		;;
esac

curl_usable() {
	command -v curl >/dev/null 2>&1 || return 1
	curl --version >/dev/null 2>&1
}

download_text() {
	local url="$1"
	if [ "$UPDATE_VIA" = "proxy" ]; then
		rm -f "$TEXT_TMP" "$TEXT_TMP.tmp"
		if [ -n "$UPDATE_OUTBOUND" ]; then
			"$ZAKOPD_BIN" download -url "$url" -output "$TEXT_TMP" -via proxy -outbound "$UPDATE_OUTBOUND" >/dev/null 2>&1 || return 1
		else
			"$ZAKOPD_BIN" download -url "$url" -output "$TEXT_TMP" -via proxy >/dev/null 2>&1 || return 1
		fi
		cat "$TEXT_TMP"
		rm -f "$TEXT_TMP"
		return 0
	fi

	if curl_usable; then
		curl -fsSL --connect-timeout 5 --max-time 12 "$url" 2>/dev/null && return 0
	fi
	if command -v wget >/dev/null 2>&1; then
		wget -q -T 12 -t 1 -O- "$url" 2>/dev/null && return 0
	fi

	return 1
}

latest_version() {
	local value=""
	local json=""

	value="$(download_text "$VERSION_URL" || true)"
	value="$(printf '%s\n' "$value" | sed -n '1{s/[[:space:]]//g;p;}')"
	if [ -z "$value" ]; then
		json="$(download_text "$RELEASE_API_URL" || true)"
		value="$(printf '%s\n' "$json" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
	fi

	[ -n "$value" ] || return 1
	release_version "$(normalize_version "$value")" || return 1
	printf '%s\n' "$value"
}

normalize_version() {
	printf '%s\n' "$1" | sed 's/^zakopd[[:space:]]*//; s/^v//; s/[-+].*$//'
}

release_version() {
	printf '%s\n' "$1" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$'
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

check_version() {
	local current=""
	local latest=""
	local current_normalized=""
	local latest_normalized=""
	local status="available"

	current="$("$ZAKOPD_BIN" version 2>/dev/null | awk '{ print $2; exit }')"
	[ -n "$current" ] || current="unknown"
	latest="$(latest_version)" || {
		echo "zakop upgrade: failed to query the latest release" >&2
		exit 1
	}

	current_normalized="$(normalize_version "$current")"
	latest_normalized="$(normalize_version "$latest")"
	if [ "$current_normalized" = "$latest_normalized" ]; then
		status="current"
	elif release_version "$current_normalized" &&
		release_version "$latest_normalized" &&
		version_ge "$current_normalized" "$latest_normalized"; then
		status="current"
	fi

	printf 'current=%s\nlatest=%s\nstatus=%s\n' "$current" "$latest" "$status"
}

download() {
	local url="$1"
	local dest="$2"
	local tmp="$dest.tmp"
	local attempts=""

	rm -f "$tmp"
	if [ "$UPDATE_VIA" = "proxy" ]; then
		if [ -n "$UPDATE_OUTBOUND" ]; then
			"$ZAKOPD_BIN" download -url "$url" -output "$dest" -via proxy -outbound "$UPDATE_OUTBOUND" && return 0
		else
			"$ZAKOPD_BIN" download -url "$url" -output "$dest" -via proxy && return 0
		fi
		echo "zakop upgrade: failed to download $url through outbound ${UPDATE_OUTBOUND:-missing}" >&2
		exit 1
	fi
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

	echo "zakop upgrade: failed to download $url; attempted:${attempts:- none}" >&2
	exit 1
}

if [ "$MODE" = "check" ]; then
	check_version
	exit 0
fi

run_upgrade() {
	local expected=""
	local actual=""

	expected="$(latest_version)" || {
		echo "zakop upgrade: failed to query the release version before installation" >&2
		return 1
	}
	download "$INSTALL_URL" "$TMP"
	if [ "$UPDATE_VIA" = "proxy" ]; then
		download "$ARCHIVE_URL" "$ARCHIVE_TMP"
		if ! ZAKOP_EXPECT_VERSION="$expected" sh "$TMP" --local "$ARCHIVE_TMP"; then
			echo "zakop upgrade: installer failed; zakopd was not updated" >&2
			return 1
		fi
	elif ! ZAKOP_EXPECT_VERSION="$expected" sh "$TMP"; then
		echo "zakop upgrade: installer failed; zakopd was not updated" >&2
		return 1
	fi
	actual="$("$ZAKOPD_BIN" version 2>/dev/null | awk '{ print $2; exit }')"
	if [ "$actual" != "$expected" ]; then
		echo "zakop upgrade: installed version $actual does not match expected $expected" >&2
		return 1
	fi
	echo "zakop upgrade: verified installed version $actual"
}

if [ "$MODE" = "luci" ]; then
	mkdir -p "$(dirname "$UPGRADE_LOG")"
	if run_upgrade >"$UPGRADE_LOG" 2>&1; then
		cat "$UPGRADE_LOG"
	else
		code="$?"
		cat "$UPGRADE_LOG" >&2
		exit "$code"
	fi
else
	run_upgrade
fi
