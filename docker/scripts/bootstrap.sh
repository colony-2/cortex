#!/usr/bin/env bash
set -euo pipefail

timestamp() {
  date -Iseconds
}

log() {
  printf '[bootstrap] %s %s\n' "$(timestamp)" "$*"
}

on_exit() {
  status=$?
  log "bootstrap exiting with status $status"
}

trap 'on_exit' EXIT
trap 'log "bootstrap received SIGTERM"; exit 143' TERM
trap 'log "bootstrap received SIGINT"; exit 130' INT

# Only run for root in interactive shells
if [ "${EUID:-$(id -u)}" -ne 0 ]; then
  exit 0
fi

DEV_UID=${DEV_UID:-1000}
PROXY_PORT=${PROXY_PORT:-8888}

log "bootstrap start (uid=${EUID:-$(id -u)}, dev_uid=$DEV_UID, proxy_port=$PROXY_PORT)"

# Ensure logs dirs exist
log "ensure log directories"
mkdir -p /var/log/supervisor /var/log/tinyproxy
chown tinyproxy:tinyproxy /var/log/tinyproxy 2>/dev/null || true

# Start supervisord (daemon mode) if not running
SUP_PIDFILE=/var/run/supervisord.pid
if ! [ -f "$SUP_PIDFILE" ] || ! kill -0 "$(cat "$SUP_PIDFILE" 2>/dev/null)" 2>/dev/null; then
  log "starting supervisord via /usr/bin/supervisord -c /etc/supervisor/supervisord-daemon.conf"
  if /usr/bin/supervisord -c /etc/supervisor/supervisord-daemon.conf; then
    log "supervisord launch returned success"
  else
    status=$?
    log "supervisord launch exited with $status"
    exit "$status"
  fi
else
  log "supervisord already running (pid $(cat "$SUP_PIDFILE" 2>/dev/null))"
fi

# Apply iptables egress restrictions for dev UID (requires NET_ADMIN)
log "applying dev-egress-setup (NET_ADMIN required)"
if /usr/local/sbin/dev-egress-setup.sh; then
  log "dev-egress-setup complete"
else
  status=$?
  log "dev-egress-setup exited with $status"
fi

sed -i 's/^nameserver .*/nameserver 127.0.0.1/' /etc/resolv.conf


log "bootstrap completed"
exit 0
