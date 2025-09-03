#!/usr/bin/env bash
set -euo pipefail

# Only run for root in interactive shells
if [ "${EUID:-$(id -u)}" -ne 0 ]; then
  exit 0
fi

DEV_UID=${DEV_UID:-1000}
PROXY_PORT=${PROXY_PORT:-8888}

# Ensure logs dirs exist
mkdir -p /var/log/supervisor /var/log/tinyproxy
chown tinyproxy:tinyproxy /var/log/tinyproxy 2>/dev/null || true

# Start supervisord (daemon mode) if not running
SUP_PIDFILE=/var/run/supervisord.pid
if ! [ -f "$SUP_PIDFILE" ] || ! kill -0 "$(cat "$SUP_PIDFILE" 2>/dev/null)" 2>/dev/null; then
  /usr/bin/supervisord -c /etc/supervisor/supervisord-daemon.conf || true
fi

# Apply iptables egress restrictions for dev UID (requires NET_ADMIN)
if command -v iptables >/dev/null 2>&1; then
  iptables -C OUTPUT -m owner --uid-owner "$DEV_UID" -p tcp -d 127.0.0.1 --dport "$PROXY_PORT" -j ACCEPT 2>/dev/null || \
    iptables -A OUTPUT -m owner --uid-owner "$DEV_UID" -p tcp -d 127.0.0.1 --dport "$PROXY_PORT" -j ACCEPT
  iptables -C OUTPUT -m owner --uid-owner "$DEV_UID" -j REJECT 2>/dev/null || \
    iptables -A OUTPUT -m owner --uid-owner "$DEV_UID" -j REJECT
fi

exit 0
