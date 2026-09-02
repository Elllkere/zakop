#!/bin/sh

set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
ARCHIVE="${1:-$ROOT_DIR/dist/zakop-openwrt-embedded.tar.gz}"
TMP="${TMPDIR:-/tmp}/zakop-archive-test.$$"

cleanup() {
	rm -rf "$TMP"
}
trap cleanup EXIT INT TERM

[ -f "$ARCHIVE" ] || {
	echo "archive not found: $ARCHIVE" >&2
	exit 1
}

mkdir -p "$TMP"
tar -xzf "$ARCHIVE" -C "$TMP"
if [ -d "$TMP/zakop" ]; then
	TMP="$TMP/zakop"
fi

for target in \
	linux-amd64 \
	linux-arm64 \
	linux-armv7 \
	linux-mips-softfloat \
	linux-mipsle-softfloat
do
	[ -x "$TMP/bin/$target/zakopd" ] || {
		echo "missing zakopd for $target" >&2
		exit 1
	}
done

[ -f "$TMP/files/etc/config/zakop" ] || {
	echo "missing default UCI config" >&2
	exit 1
}
[ -x "$TMP/install.sh" ] || {
	echo "missing install.sh" >&2
	exit 1
}
[ -x "$TMP/uninstall.sh" ] || {
	echo "missing uninstall.sh" >&2
	exit 1
}
[ -x "$TMP/upgrade.sh" ] || {
	echo "missing upgrade.sh" >&2
	exit 1
}
[ -s "$TMP/zakop-version.txt" ] || {
	echo "missing zakop-version.txt" >&2
	exit 1
}
[ -s "$TMP/zakop-ui-cache.txt" ] || {
	echo "missing LuCI cache namespace" >&2
	exit 1
}
ui_namespace="$(sed -n '1{s/[[:space:]]//g;p;}' "$TMP/zakop-ui-cache.txt")"
case "$ui_namespace" in
	zakop_[0-9]*) ;;
	*)
		echo "invalid LuCI cache namespace: $ui_namespace" >&2
		exit 1
		;;
esac
[ -f "$TMP/files/www/luci-static/resources/view/$ui_namespace/outbounds.js" ] || {
	echo "missing versioned LuCI outbounds view" >&2
	exit 1
}
[ -f "$TMP/files/www/luci-static/resources/$ui_namespace/i18n.js" ] || {
	echo "missing versioned LuCI helper modules" >&2
	exit 1
}
[ -s "$TMP/files/www/luci-static/resources/zakop-assets/TwemojiCountryFlags-0.1.8.woff2" ] || {
	echo "missing LuCI country flag emoji font" >&2
	exit 1
}
[ -s "$TMP/files/www/luci-static/resources/zakop-assets/LICENSE.md" ] || {
	echo "missing LuCI country flag emoji font license" >&2
	exit 1
}
grep -Fq "\"path\": \"$ui_namespace/outbounds\"" \
	"$TMP/files/usr/share/luci/menu.d/luci-app-zakop.json" || {
	echo "LuCI menu does not reference versioned outbounds view" >&2
	exit 1
}
expected_version="$(sed -n '1{s/[[:space:]]//g;p;}' "$TMP/zakop-version.txt")"
actual_version="$("$TMP/bin/linux-amd64/zakopd" version | awk '{ print $2; exit }')"
[ "$actual_version" = "$expected_version" ] || {
	echo "archive version mismatch: manifest=$expected_version zakopd=$actual_version" >&2
	exit 1
}
[ -x "$TMP/files/usr/share/zakop/check-version.sh" ] || {
	echo "missing check-version.sh" >&2
	exit 1
}

echo "archive ok"
