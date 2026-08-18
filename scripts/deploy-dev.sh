#!/bin/sh

set -eu

: "${ZAKOP_ROUTER:?set ZAKOP_ROUTER to root@router-address}"

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
BIN="$ROOT_DIR/dist/zakopd-dev"

if [ ! -x "$BIN" ]; then
	"$ROOT_DIR/scripts/build-dev.sh" >/dev/null
fi

scp "$BIN" "$ZAKOP_ROUTER:/usr/bin/zakopd"
ssh "$ZAKOP_ROUTER" "chmod 0755 /usr/bin/zakopd && /etc/init.d/zakop restart"

