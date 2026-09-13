#!/bin/sh

set -eu

PURGE=0
rm -f /etc/hotplug.d/iface/95-zakop-debug
if [ "${1:-}" = "--purge" ]; then
	PURGE=1
fi

if [ -x /etc/init.d/zakop ]; then
	/etc/init.d/zakop stop || true
	/etc/init.d/zakop disable || true
fi

dns_listen="$(uci -q get zakop.main.dns_listen || echo '127.0.0.1:5353')"
dns_host="${dns_listen%:*}"
dns_port="${dns_listen##*:}"
dns_server="$dns_host#$dns_port"
state_dir="/etc/zakop/dnsmasq-state"
legacy_state_dir="/var/lib/zakop"
server_state="$state_dir/dnsmasq-server.prev"
legacy_server_state="$legacy_state_dir/dnsmasq-server.prev"
noresolv_state="$state_dir/dnsmasq-noresolv.prev"
legacy_noresolv_state="$legacy_state_dir/dnsmasq-noresolv.prev"
addsubnet_state="$state_dir/dnsmasq-addsubnet.prev"
legacy_addsubnet_state="$legacy_state_dir/dnsmasq-addsubnet.prev"

read_state() {
	if [ -f "$1" ]; then
		cat "$1"
		return 0
	fi
	if [ -f "$2" ]; then
		cat "$2"
		return 0
	fi
	return 1
}

is_zakop_dnsmasq_server() {
	value="$1"
	current="$2"
	saved="$3"

	[ -n "$current" ] && [ "$value" = "$current" ] && return 0
	[ -n "$saved" ] && [ "$value" = "$saved" ] && return 0
	[ "$value" = "127.0.0.1#5353" ] && return 0
	return 1
}

has_non_zakop_dnsmasq_server() {
	current="$1"
	saved="$2"
	servers="$(uci -q get dhcp.@dnsmasq[0].server || true)"
	for value in $servers; do
		if ! is_zakop_dnsmasq_server "$value" "$current" "$saved"; then
			return 0
		fi
	done
	return 1
}

saved_server="$(read_state "$server_state" "$legacy_server_state" 2>/dev/null || true)"
uci -q del_list dhcp.@dnsmasq[0].server="$dns_server" || true
if [ -n "$saved_server" ] && [ "$saved_server" != "$dns_server" ]; then
	uci -q del_list dhcp.@dnsmasq[0].server="$saved_server" || true
fi
if [ "$dns_server" != "127.0.0.1#5353" ] && [ "$saved_server" != "127.0.0.1#5353" ]; then
	uci -q del_list dhcp.@dnsmasq[0].server="127.0.0.1#5353" || true
fi
if old_noresolv="$(read_state "$noresolv_state" "$legacy_noresolv_state" 2>/dev/null)"; then
	if [ "$old_noresolv" = "__missing__" ]; then
		uci -q delete dhcp.@dnsmasq[0].noresolv || true
	else
		uci set dhcp.@dnsmasq[0].noresolv="$old_noresolv"
	fi
fi
if old_addsubnet="$(read_state "$addsubnet_state" "$legacy_addsubnet_state" 2>/dev/null)"; then
	if [ "$old_addsubnet" = "__missing__" ]; then
		uci -q delete dhcp.@dnsmasq[0].addsubnet || true
	else
		uci set dhcp.@dnsmasq[0].addsubnet="$old_addsubnet"
	fi
fi
current_noresolv="$(uci -q get dhcp.@dnsmasq[0].noresolv || true)"
if [ "$current_noresolv" = "1" ] && ! has_non_zakop_dnsmasq_server "$dns_server" "$saved_server"; then
	uci -q delete dhcp.@dnsmasq[0].noresolv || true
fi
current_addsubnet="$(uci -q get dhcp.@dnsmasq[0].addsubnet || true)"
if [ "$current_addsubnet" = "32" ] && ! has_non_zakop_dnsmasq_server "$dns_server" "$saved_server"; then
	uci -q delete dhcp.@dnsmasq[0].addsubnet || true
fi
uci commit dhcp || true

rm -f /etc/init.d/zakop
rm -f /usr/bin/zakopd
rm -rf /usr/libexec/zakop
rm -rf /usr/share/zakop
rm -f /usr/share/luci/menu.d/luci-app-zakop.json
rm -f /usr/share/rpcd/acl.d/luci-app-zakop.json
for path in \
	/www/luci-static/resources/view/zakop \
	/www/luci-static/resources/view/zakop_* \
	/www/luci-static/resources/zakop \
	/www/luci-static/resources/zakop_* \
	/www/luci-static/resources/zakop-assets
do
	[ -d "$path" ] && rm -rf "$path"
done
rm -rf /tmp/zakop
rm -rf /var/lib/zakop
rm -rf /etc/zakop/dnsmasq-state
rm -f /tmp/dnsmasq.d/zakop.conf /etc/dnsmasq.d/zakop.conf

if [ "$PURGE" -eq 1 ]; then
	rm -f /etc/config/zakop
	rm -rf /etc/zakop
fi

if [ -x /etc/init.d/rpcd ]; then
	/etc/init.d/rpcd restart || true
fi
if [ -x /etc/init.d/uhttpd ]; then
	/etc/init.d/uhttpd restart || true
fi
if [ -x /etc/init.d/dnsmasq ]; then
	/etc/init.d/dnsmasq reload || /etc/init.d/dnsmasq restart || true
fi

echo "zakop uninstalled"
