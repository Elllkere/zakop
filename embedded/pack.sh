#!/bin/sh

set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
OUT_DIR="${OUT_DIR:-$ROOT_DIR/dist}"
WORK_DIR="${TMPDIR:-/tmp}/zakop-pack.$$"
ARCHIVE_ROOT="$WORK_DIR/zakop"
VERSION="${ZAKOP_VERSION:-$(git -C "$ROOT_DIR" describe --tags --always --dirty 2>/dev/null || echo dev)}"
BUILD_TAGS=""
BUILD_FLAVOR="production"
ARCHIVE="$OUT_DIR/zakop-openwrt-embedded.tar.gz"
case "${1:-}" in
	"") ;;
	--network-debug)
		BUILD_TAGS="networkdebug"
		BUILD_FLAVOR="network-debug"
		ARCHIVE="$OUT_DIR/zakop-openwrt-network-debug.tar.gz"
		VERSION="$VERSION-network-debug"
		;;
	*) echo "usage: $0 [--network-debug]" >&2; exit 1 ;;
esac
[ "$#" -le 1 ] || { echo "too many arguments" >&2; exit 1; }

cleanup() {
	rm -rf "$WORK_DIR"
}
trap cleanup EXIT INT TERM

build_zakopd() {
	target="$1"
	goarch="$2"
	goarm="$3"
	gomips="$4"
	dest="$ARCHIVE_ROOT/bin/$target/zakopd"

	mkdir -p "$(dirname "$dest")"
	echo "building $target"
	(
		cd "$ROOT_DIR"
		GOOS=linux \
		GOARCH="$goarch" \
		GOARM="$goarm" \
		GOMIPS="$gomips" \
		CGO_ENABLED=0 \
		go build -tags "$BUILD_TAGS" -buildvcs=false -trimpath -ldflags "-s -w -X main.version=$VERSION" -o "$dest" ./cmd/zakopd
	)
	chmod 0755 "$dest"
}

copy_managed_singbox() {
	target="$1"
	src="$ROOT_DIR/embedded/bin/$target/sing-box"
	dest="$ARCHIVE_ROOT/bin/$target/sing-box"
	if [ -x "$src" ]; then
		cp "$src" "$dest"
		chmod 0755 "$dest"
	fi
}

rm -rf "$WORK_DIR"
mkdir -p "$ARCHIVE_ROOT" "$OUT_DIR"

cp -R "$ROOT_DIR/embedded/files" "$ARCHIVE_ROOT/files"
cp "$ROOT_DIR/embedded/install.sh" "$ARCHIVE_ROOT/install.sh"
cp "$ROOT_DIR/embedded/uninstall.sh" "$ARCHIVE_ROOT/uninstall.sh"
cp "$ROOT_DIR/embedded/upgrade.sh" "$ARCHIVE_ROOT/upgrade.sh"
if [ "$BUILD_FLAVOR" = "network-debug" ]; then
	cp -R "$ROOT_DIR/embedded/networkdebug/files/." "$ARCHIVE_ROOT/files/"
	mkdir -p "$ARCHIVE_ROOT/diagnostics"
	cp "$ROOT_DIR/scripts/dnat-stress.py" "$ROOT_DIR/scripts/test-dnat-netns.py" \
		"$ROOT_DIR/scripts/router-network-check.sh" "$ROOT_DIR/docs/NETWORK_AUDIT.md" \
		"$ROOT_DIR/docs/NETWORK_DIAGNOSTICS.md" "$ROOT_DIR/docs/NETWORK_VALIDATION.md" "$ARCHIVE_ROOT/diagnostics/"
fi
# Strip entire diagnostic shell blocks from production, not runtime branches.
for script in "$ARCHIVE_ROOT/files/etc/init.d/zakop" "$ARCHIVE_ROOT/files/usr/share/zakop/run-sing-box-log.sh"; do
	awk -v flavor="$BUILD_FLAVOR" '
		/# BEGIN NETWORKDEBUG/ { skip = (flavor == "production"); next }
		/# END NETWORKDEBUG/ { skip = 0; next }
		!skip { print }
	' "$script" > "$script.rendered"
	# Preserve executable permissions inherited from the source file.
	cat "$script.rendered" > "$script"
	rm -f "$script.rendered"
done
printf '%s\n' "$BUILD_FLAVOR" > "$ARCHIVE_ROOT/zakop-build-flavor.txt"
printf '%s\n' "$VERSION" > "$ARCHIVE_ROOT/zakop-version.txt"
chmod 0755 "$ARCHIVE_ROOT/install.sh" "$ARCHIVE_ROOT/uninstall.sh" "$ARCHIVE_ROOT/upgrade.sh"

# Base LuCI's resource version does not change when this standalone app is
# upgraded. Give each distinct zakop UI build a content-derived module namespace
# so browsers cannot reuse stale view or helper scripts.
UI_VIEW_DIR="$ARCHIVE_ROOT/files/www/luci-static/resources/view/zakop"
UI_HELPER_DIR="$ARCHIVE_ROOT/files/www/luci-static/resources/zakop"
UI_CACHE_KEY="$({
	for file in "$UI_VIEW_DIR"/*.js "$UI_HELPER_DIR"/*.js; do
		cksum "$file" | awk '{ print $1 ":" $2 }'
	done
} | cksum | awk '{ print $1 }')"
UI_NAMESPACE="zakop_${UI_CACHE_KEY}"
mv "$UI_VIEW_DIR" "$ARCHIVE_ROOT/files/www/luci-static/resources/view/$UI_NAMESPACE"
mv "$UI_HELPER_DIR" "$ARCHIVE_ROOT/files/www/luci-static/resources/$UI_NAMESPACE"
for file in "$ARCHIVE_ROOT/files/www/luci-static/resources/view/$UI_NAMESPACE"/*.js; do
	sed -i "s/'require zakop\./'require $UI_NAMESPACE./g" "$file"
done
sed -i "s#\"path\": \"zakop/#\"path\": \"$UI_NAMESPACE/#g" \
	"$ARCHIVE_ROOT/files/usr/share/luci/menu.d/luci-app-zakop.json"
printf '%s\n' "$UI_NAMESPACE" > "$ARCHIVE_ROOT/zakop-ui-cache.txt"

build_zakopd "linux-amd64" "amd64" "" ""
build_zakopd "linux-arm64" "arm64" "" ""
build_zakopd "linux-armv7" "arm" "7" ""
build_zakopd "linux-mips-softfloat" "mips" "" "softfloat"
build_zakopd "linux-mipsle-softfloat" "mipsle" "" "softfloat"

copy_managed_singbox "linux-amd64"
copy_managed_singbox "linux-arm64"
copy_managed_singbox "linux-armv7"
copy_managed_singbox "linux-mips-softfloat"
copy_managed_singbox "linux-mipsle-softfloat"

tar -C "$WORK_DIR" -czf "$ARCHIVE" zakop
echo "$ARCHIVE"
