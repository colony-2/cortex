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
/usr/local/sbin/dev-egress-setup.sh || true

exit 0
