#!/bin/sh

set -eu

AWS_IP_RANGES_URL="https://ip-ranges.amazonaws.com/ip-ranges.json"

AWS_SERVICES="
CLOUDFRONT
S3
AMAZON
EC2
GLOBALACCELERATOR
"

WORK_DIR="${TMPDIR:-/tmp}/zakop-aws-full-ipv4.$$"
JSON_FILE="$WORK_DIR/ip-ranges.json"
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

extract_aws_prefixes() {
	if command -v jq >/dev/null 2>&1; then
		jq -r --arg services "$AWS_SERVICES" '
			($services | split("\n") | map(select(length > 0))) as $allowed_services |
			.prefixes[] |
			select(
				(.region | startswith("eu-")) and
				(.service as $s | $allowed_services | index($s))
			) |
			.ip_prefix
		' | grep -E '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+/[0-9]+$' || true
		return
	fi

	awk -v services="$AWS_SERVICES" '
	BEGIN {
		split(services, serviceList)
		for (i in serviceList) {
			if (serviceList[i] != "")
				allowedService[serviceList[i]] = 1
		}
	}
	function json_value(line, value) {
		value = line
		sub(/^[^:]*:[[:space:]]*"/, "", value)
		sub(/".*$/, "", value)
		return value
	}
	function reset_entry() {
		ip = ""
		region = ""
		service = ""
	}
	function emit_entry() {
		if (ip != "" && region ~ /^eu-/ && allowedService[service])
			print ip
	}
	/^[[:space:]]*\{/ {
		reset_entry()
	}
	/"ip_prefix"[[:space:]]*:/ {
		ip = json_value($0)
	}
	/"region"[[:space:]]*:/ {
		region = json_value($0)
	}
	/"service"[[:space:]]*:/ {
		service = json_value($0)
	}
	/^[[:space:]]*\}/ {
		emit_entry()
		reset_entry()
	}
	'
}

mkdir -p "$WORK_DIR"

echo "zakop: fetching AWS Full IPv4 ranges (AMAZON, EC2, GLOBALACCELERATOR)" >&2
echo "zakop: warning: routing AWS Full may affect ping to games hosted on Amazon/AWS servers" >&2
fetch_url "$AWS_IP_RANGES_URL" > "$JSON_FILE"
extract_aws_prefixes < "$JSON_FILE" | sort -u > "$RESULT_FILE"

if [ ! -s "$RESULT_FILE" ]; then
	echo "zakop: AWS Full IPv4 provider returned an empty list" >&2
	exit 1
fi

if [ -n "${ZAKOP_PROVIDER_OUTPUT:-}" ]; then
	cp "$RESULT_FILE" "$ZAKOP_PROVIDER_OUTPUT"
else
	cat "$RESULT_FILE"
fi
