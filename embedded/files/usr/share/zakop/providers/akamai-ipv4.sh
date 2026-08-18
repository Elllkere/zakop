#!/bin/sh

set -eu

AKAMAI_ASNS="
AS20940
AS16625
AS35994
AS36183
AS63949
AS32787
"

WORK_DIR="${TMPDIR:-/tmp}/zakop-akamai-ipv4.$$"
LIST_FILE="$WORK_DIR/prefixes.txt"
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

extract_ripe_prefixes() {
	if command -v jq >/dev/null 2>&1; then
		jq -r '.data.prefixes[]?.prefix // empty' \
			| grep -E '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+/[0-9]+$' || true
		return
	fi

	awk '
	{
		line = $0
		while (match(line, /"prefix"[[:space:]]*:[[:space:]]*"[0-9][0-9.]*\/[0-9][0-9]*"/)) {
			item = substr(line, RSTART, RLENGTH)
			sub(/^"prefix"[[:space:]]*:[[:space:]]*"/, "", item)
			sub(/"$/, "", item)
			print item
			line = substr(line, RSTART + RLENGTH)
		}
	}'
}

mkdir -p "$WORK_DIR"
: > "$LIST_FILE"

for asn in $AKAMAI_ASNS; do
	url="https://stat.ripe.net/data/announced-prefixes/data.json?resource=${asn}"
	json_file="$WORK_DIR/${asn}.json"
	echo "zakop: fetching Akamai prefixes for ${asn}" >&2
	fetch_url "$url" > "$json_file"
	extract_ripe_prefixes < "$json_file" >> "$LIST_FILE"
done

sort -u "$LIST_FILE" > "$RESULT_FILE"

if [ ! -s "$RESULT_FILE" ]; then
	echo "zakop: Akamai IPv4 provider returned an empty list" >&2
	exit 1
fi

if [ -n "${ZAKOP_PROVIDER_OUTPUT:-}" ]; then
	cp "$RESULT_FILE" "$ZAKOP_PROVIDER_OUTPUT"
else
	cat "$RESULT_FILE"
fi
