#!/usr/bin/env bash
set -euo pipefail

# Watches the allowlist file and rebuilds dnsmasq config, then HUPs dnsmasq and tinyproxy.
# Works with bind mounts by combining inotify (if available) with a polling fallback.

ALLOWLIST_FILE=${ALLOWLIST_FILE:-/etc/shai/allowed_domains.conf}
OUT_FILE=${OUT_FILE:-/etc/dnsmasq.d/allowlist.conf}
POLL_INTERVAL=${POLL_INTERVAL:-1}
LOCK_FILE=${LOCK_FILE:-/var/run/allowlist-rebuild.lock}

mkdir -p /var/run || true

wait_for_allowlist() {
  # Wait briefly for editors that write via temp+rename or delayed syncs
  local tries=30
  local i size1 size2
  for ((i=0; i<tries; i++)); do
    if [ -r "${ALLOWLIST_FILE}" ]; then
      size1=$(stat -c %s "${ALLOWLIST_FILE}" 2>/dev/null || echo -1)
      sleep 0.1
      size2=$(stat -c %s "${ALLOWLIST_FILE}" 2>/dev/null || echo -2)
      if [ "$size1" = "$size2" ] && [ "$size1" -ge 0 ]; then
        return 0
      fi
    fi
    sleep 0.1
  done
  return 1
}

rebuild() {
  # Ensure target file is present and stable before regenerating
  if ! wait_for_allowlist; then
    echo "[allowlist-watcher] allowlist not present/stable; skipping"
    return 0
  fi
  chmod a+r "${ALLOWLIST_FILE}" 2>/dev/null || true
  # Serialize concurrent triggers
  exec 9>"${LOCK_FILE}" || true
  if flock -n 9; then
    /usr/local/sbin/generate-dnsmasq-allowlist.sh "${ALLOWLIST_FILE}" "${OUT_FILE}" || true
    pkill -HUP -x dnsmasq 2>/dev/null || true
    pkill -HUP -x tinyproxy 2>/dev/null || true
  fi
}

# Initial build at start
if [ -f "${ALLOWLIST_FILE}" ]; then
  rebuild
fi

# Background polling fallback to handle bind mounts where inotify may not fire
prev_hash=""
(
  while true; do
    if [ -f "${ALLOWLIST_FILE}" ]; then
      curr_hash=$(sha256sum "${ALLOWLIST_FILE}" 2>/dev/null | awk '{print $1}')
      if [ -n "${curr_hash}" ] && [ "${curr_hash}" != "${prev_hash}" ]; then
        prev_hash="${curr_hash}"
        rebuild
      fi
    fi
    sleep "${POLL_INTERVAL}"
  done
) &

# Inotify watcher (best-effort)
if command -v inotifywait >/dev/null 2>&1; then
  dir=$(dirname "${ALLOWLIST_FILE}")
  base=$(basename "${ALLOWLIST_FILE}")
  # Monitor directory and filter on target filename
  while read -r watch_path events filename; do
    if [ "${filename:-}" = "${base}" ]; then
      rebuild
    fi
  done < <(inotifywait -m -e close_write,move,create,attrib "${dir}" 2>/dev/null)
else
  # If no inotify, just wait on polling loop
  wait
fi
