#!/bin/sh
# Short RPC calls only: the worker never inherits rpcd's input/output pipes.
set -eu
umask 077
root=/tmp/zakop-luci-jobs

validate() {
	case "$1" in
		restart|import) [ "$#" -eq 1 ] ;;
		subscriptions|providers) [ "$#" -eq 2 ] && case "$2" in ''|*[!a-zA-Z0-9_]*) return 1 ;; esac ;;
		latency) [ "$#" -le 2 ] && case "${2:-all}" in -*) return 1 ;; esac ;;
		*) return 1 ;;
	esac
}

case "${1:-}" in
	start)
		shift
		validate "$@" || exit 2
		mkdir -p "$root"
		# Abandoned browser sessions must not accumulate files indefinitely.
		find "$root" -mindepth 1 -maxdepth 1 -type d -mtime +0 -exec rm -rf '{}' \;
		job=$(mktemp -d "$root/job.XXXXXXXX")
		"$0" run "${job##*/}" "$@" </dev/null >/dev/null 2>&1 &
		echo "${job##*/}"
		;;
	run)
		id=$2
		shift 2
		case "$id" in job.*) ;; *) exit 2 ;; esac
		case "$id" in *[!a-zA-Z0-9.]*|*..*) exit 2 ;; esac
		validate "$@" || exit 2
		job=$root/$id
		[ "$1" != latency ] || touch "$job/progress"
		trap 'echo 125 >"$job/code"' HUP INT TERM
		set +e
		case "$1" in
			restart) /etc/init.d/zakop restart ;;
			import) /usr/bin/zakopd import-uri -file /tmp/zakop-import.txt ;;
			latency) /usr/bin/zakopd outbounds latency -progress ${2:+"$2"} ;;
			*) /usr/bin/zakopd "$1" update "$2" ;;
		esac >"$job/stdout" 2>"$job/stderr"
		code=$?
		echo "$code" >"$job/code.tmp"
		mv "$job/code.tmp" "$job/code"
		;;
	status|status-direct|remove)
		[ "$#" -eq 2 ] || exit 2
		case "$2" in job.*) ;; *) exit 2 ;; esac
		case "$2" in *[!a-zA-Z0-9.]*|*..*) exit 2 ;; esac
		job=$root/$2
		[ -d "$job" ] || exit 2
		if [ "$1" = remove ]; then
			[ -f "$job/code" ] && rm -rf "$job"
			exit 0
		fi
		if [ -f "$job/code" ]; then
			printf 'exit:%s\n' "$(cat "$job/code")"
		else
			echo running
		fi
		if [ -f "$job/progress" ]; then
			# Include a complete snapshot even if the next line is being written.
			tail -n 2 "$job/stdout" 2>/dev/null || true
		else
			cat "$job/stdout" 2>/dev/null || true
		fi
		if [ "$1" = status-direct ]; then
			# CGI returns stdout rather than rpcd's {stdout, stderr} object.
			printf '\n__ZAKOP_JOB_STDERR__\n'
			cat "$job/stderr" 2>/dev/null || true
		else
			cat "$job/stderr" >&2 2>/dev/null || true
		fi
		;;
	*) exit 2 ;;
esac
