#!/bin/sh

bin="${1:-/usr/libexec/zakop/sing-box}"
config="${2:-/tmp/zakop/sing-box.json}"
zakopd_bin="${ZAKOP_ZAKOPD:-/usr/bin/zakopd}"
log_file="${ZAKOP_SINGBOX_LOG:-/tmp/zakop/sing-box.log}"
log_max_bytes="${ZAKOP_SINGBOX_LOG_MAX_BYTES:-524288}"
log_keep_bytes="${ZAKOP_SINGBOX_LOG_KEEP_BYTES:-262144}"
health_grace="${ZAKOP_DNS_HEALTH_GRACE_SECONDS:-30}"
health_interval="${ZAKOP_DNS_HEALTH_INTERVAL_SECONDS:-10}"
health_failures_max="${ZAKOP_DNS_HEALTH_FAILURES:-3}"
health_timeout="${ZAKOP_DNS_HEALTH_TIMEOUT_SECONDS:-3}"
log_dir="${log_file%/*}"
child_pid=""
log_enabled=0

rotate_log() {
	case "$log_max_bytes" in
		""|*[!0-9]*)
			log_max_bytes="524288"
			;;
	esac
	case "$log_keep_bytes" in
		""|*[!0-9]*)
			log_keep_bytes="262144"
			;;
	esac

	size="$(wc -c < "$log_file" 2>/dev/null || printf 0)"
	case "$size" in
		""|*[!0-9]*)
			return 0
			;;
	esac
	[ "$size" -gt "$log_max_bytes" ] || return 0

	tmp="${log_file}.$$"
	if tail -c "$log_keep_bytes" "$log_file" > "$tmp" 2>/dev/null; then
		mv "$tmp" "$log_file"
	else
		rm -f "$tmp"
	fi
}

positive_number() {
	case "$1" in
		""|*[!0-9]*|0)
			return 1
			;;
	esac
	return 0
}

positive_number "$health_grace" || health_grace=30
positive_number "$health_interval" || health_interval=10
positive_number "$health_failures_max" || health_failures_max=3
positive_number "$health_timeout" || health_timeout=3

log_message() {
	[ "$log_enabled" -eq 1 ] || return 0
	printf "%s zakop: %s\n" "$(date '+%Y-%m-%d %H:%M:%S')" "$*" >> "$log_file"
}

start_singbox() {
	log_message "starting sing-box"
	if [ "$log_enabled" -eq 1 ]; then
		"$bin" run -c "$config" >> "$log_file" 2>&1 &
	else
		"$bin" run -c "$config" >/dev/null 2>&1 &
	fi
	child_pid=$!
}

child_running() {
	local state

	[ -n "$child_pid" ] && kill -0 "$child_pid" 2>/dev/null || return 1
	if [ -r "/proc/$child_pid/stat" ]; then
		state="$(awk '{ print $3 }' "/proc/$child_pid/stat" 2>/dev/null || true)"
		[ "$state" = "Z" ] && return 1
	fi
	return 0
}

wait_with_child() {
	local remaining="$1"

	while [ "$remaining" -gt 0 ]; do
		child_running || return 1
		sleep 1
		remaining=$((remaining - 1))
	done
	return 0
}

stop_child() {
	trap - INT TERM
	if [ -n "$child_pid" ]; then
		child_running && kill "$child_pid" 2>/dev/null || true
		wait "$child_pid" 2>/dev/null || true
	fi
	exit 0
}

trap stop_child INT TERM

if mkdir -p "$log_dir" 2>/dev/null && touch "$log_file" 2>/dev/null; then
	log_enabled=1
	chmod 0644 "$log_file" 2>/dev/null || true
	rotate_log
fi

while :; do
	start_singbox
	if ! wait_with_child "$health_grace"; then
		wait "$child_pid"
		exit $?
	fi

	health_failures=0
	restart_for_health=0
	while child_running; do
		if "$zakopd_bin" ready -timeout "${health_timeout}s" >/dev/null 2>&1; then
			health_failures=0
		else
			health_failures=$((health_failures + 1))
			if [ "$health_failures" -ge "$health_failures_max" ]; then
				log_message "real DNS health check failed $health_failures times; restarting sing-box"
				restart_for_health=1
				kill "$child_pid" 2>/dev/null || true
				wait "$child_pid" 2>/dev/null || true
				child_pid=""
				break
			fi
		fi
		wait_with_child "$health_interval" || break
	done

	if [ "$restart_for_health" -eq 1 ]; then
		sleep 2
		continue
	fi

	wait "$child_pid"
	exit $?
done
