#!/bin/sh
# Run only on the test router. Default: read-only network checks.
set -eu
if [ ! -f /etc/openwrt_release ] || [ ! -x /etc/init.d/zakop ]; then
	echo "Run on the test OpenWrt/ImmortalWrt router, not the development host." >&2
	exit 1
fi
mode="${1:-check}"
case "$mode" in check|lifecycle) ;; *) echo "usage: $0 [check|lifecycle]" >&2; exit 1 ;; esac
work="$(mktemp -d /tmp/zakop-network-check.XXXXXX)"
cleanup() {
	rm -f "$work/zakop.nft" "$work/sing-box.json" "$work/rules.before" "$work/rules.after"
	rmdir "$work" 2>/dev/null || true
}
trap cleanup EXIT

check_state() {
	# Avoid rewriting the active runtime JSON during validation.
	/usr/bin/zakopd check -out-dir "$work"
	/usr/bin/zakopd ready -timeout 30s
	ip -4 rule show
	table="$(uci -q get zakop.main.table || echo 101)"
	ip -4 route show table "$table"
	nft list chain inet zakop prerouting
	nft list chain inet zakop from_lan
	pidof sing-box || true
	pidof zakopd || true
	if command -v ss >/dev/null 2>&1; then ss -s; ss -lnptu; fi
}

if [ "$mode" = lifecycle ]; then
	echo "Lifecycle test interrupts proxy/DNS traffic. Use a local console and a maintenance window."
	for iteration in 1 2; do
		echo "Lifecycle iteration $iteration"
		/etc/init.d/zakop stop
		if nft list table inet zakop >/dev/null 2>&1; then
			echo "FAIL: Zakop table survived stop" >&2; exit 1
		fi
		/etc/init.d/zakop stop
		/etc/init.d/zakop start
		check_state
		ip -4 rule show > "$work/rules.before"
		/etc/init.d/zakop start
		/etc/init.d/zakop reload
		check_state
		ip -4 rule show > "$work/rules.after"
		cmp "$work/rules.before" "$work/rules.after" || {
			echo "FAIL: RPDB changed after repeated start/reload; inspect priority/duplicates" >&2; exit 1
		}
		/etc/init.d/zakop restart
		check_state
	done
else
	check_state
fi

echo "Runtime checks completed. Compare rule counts/PIDs with your expected targets."
