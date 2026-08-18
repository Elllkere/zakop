#!/bin/sh

set -eu

GOOGLE_CLOUD_RANGES_URL="https://www.gstatic.com/ipranges/cloud.json"
WORK_DIR="${TMPDIR:-/tmp}/zakop-google-cloud-eu-ipv4.$$"
JSON_FILE="$WORK_DIR/cloud.json"
RESULT_FILE="$WORK_DIR/result.txt"

cleanup() {
	rm -rf "$WORK_DIR"
}
trap cleanup EXIT INT TERM

fetch_url() {
	url="$1"

	if [ -n "${ZAKOP_PROVIDER_PROXY:-}" ]; then
		curl -fsSL --connect-timeout 15 --max-time 60 --proxy "$ZAKOP_PROVIDER_PROXY" "$url"
	else
		curl -fsSL --connect-timeout 15 --max-time 60 --noproxy "*" "$url"
	fi
}

extract_google_cloud_prefixes() {
	if command -v jq >/dev/null 2>&1; then
		jq -r '
			.prefixes[] |
			select(.scope | startswith("europe-")) |
			.ipv4Prefix // empty
		' | grep -E '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+/[0-9]+$' || true
		return
	fi

	awk '
	function json_value(line, value) {
		value = line
		sub(/^[^:]*:[[:space:]]*"/, "", value)
		sub(/".*$/, "", value)
		return value
	}
	function reset_entry() {
		ip = ""
		scope = ""
	}
	function emit_entry() {
		if (scope ~ /^europe-/ && ip ~ /^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+\/[0-9]+$/)
			print ip
	}
	/^[[:space:]]*\{/ {
		reset_entry()
	}
	/"ipv4Prefix"[[:space:]]*:/ {
		ip = json_value($0)
	}
	/"scope"[[:space:]]*:/ {
		scope = json_value($0)
	}
	/^[[:space:]]*\}/ {
		emit_entry()
		reset_entry()
	}
	'
}

mkdir -p "$WORK_DIR"

echo "zakop: fetching Google Cloud IPv4 ranges for Europe" >&2
fetch_url "$GOOGLE_CLOUD_RANGES_URL" > "$JSON_FILE"
extract_google_cloud_prefixes < "$JSON_FILE" | sort -u > "$RESULT_FILE"

if [ ! -s "$RESULT_FILE" ]; then
	echo "zakop: Google Cloud Europe IPv4 provider returned an empty list" >&2
	exit 1
fi

if [ -n "${ZAKOP_PROVIDER_OUTPUT:-}" ]; then
	cp "$RESULT_FILE" "$ZAKOP_PROVIDER_OUTPUT"
else
	cat "$RESULT_FILE"
fi
