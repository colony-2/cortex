#!/usr/bin/env bash
set -euo pipefail

ALLOWLIST_FILE=${1:-/etc/shai/allowed_domains.conf}
OUT_FILE=${2:-/etc/dnsmasq.d/allowlist.conf}
UPSTREAM4=${UPSTREAM4:-1.1.1.1}
UPSTREAM4_ALT=${UPSTREAM4_ALT:-9.9.9.9}
UPSTREAM6=${UPSTREAM6:-2606:4700:4700::1111}
UPSTREAM6_ALT=${UPSTREAM6_ALT:-2620:fe::9}

tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT

echo "# Generated from $ALLOWLIST_FILE on $(date -u +%FT%TZ)" > "$tmp"
echo "# Forward only listed domains; all others have no upstream and will SERVFAIL" >> "$tmp"

# Normalize domains: strip protocol, trim, drop leading dots and wildcards
{
  while IFS= read -r raw || [ -n "$raw" ]; do
    # Strip comments and surrounding whitespace
    line="${raw%%#*}"; line="${line%%$'\r'}"; line="${line##*[[:space:]]}"
    # Trim leading/trailing whitespace using awk fallback
    line=$(printf '%s' "$line" | awk '{gsub(/^\s+|\s+$/, ""); print}')
    [ -z "$line" ] && continue
    # Normalize: remove schema, lower-case, strip leading dot or wildcard.
    d="$line"
    d="${d#http://}"; d="${d#https://}"
    d="${d#.}"; d="${d#*.}"
    d=$(printf '%s' "$d" | tr 'A-Z' 'a-z')
    [ -z "$d" ] && continue
    printf "server=/%s/%s\n" "$d" "$UPSTREAM4" >> "$tmp"
    printf "server=/%s/%s\n" "$d" "$UPSTREAM4_ALT" >> "$tmp"
    printf "server=/%s/%s\n" "$d" "$UPSTREAM6" >> "$tmp"
    printf "server=/%s/%s\n" "$d" "$UPSTREAM6_ALT" >> "$tmp"
  done < "$ALLOWLIST_FILE"
}

install -m 0644 -D "$tmp" "$OUT_FILE"
echo "Wrote dnsmasq allowlist to $OUT_FILE"
