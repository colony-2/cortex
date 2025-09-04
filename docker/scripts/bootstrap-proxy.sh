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
  # DNS local resolver
  # Redirect all devuser DNS to local dnsmasq, regardless of /etc/resolv.conf
  iptables -t nat -C OUTPUT -m owner --uid-owner "$DEV_UID" -p udp --dport 53 -j REDIRECT --to-ports 53 2>/dev/null || \
    iptables -t nat -A OUTPUT -m owner --uid-owner "$DEV_UID" -p udp --dport 53 -j REDIRECT --to-ports 53
  iptables -t nat -C OUTPUT -m owner --uid-owner "$DEV_UID" -p tcp --dport 53 -j REDIRECT --to-ports 53 2>/dev/null || \
    iptables -t nat -A OUTPUT -m owner --uid-owner "$DEV_UID" -p tcp --dport 53 -j REDIRECT --to-ports 53
  iptables -C OUTPUT -m owner --uid-owner "$DEV_UID" -p udp -d 127.0.0.1 --dport 53 -j ACCEPT 2>/dev/null || \
    iptables -A OUTPUT -m owner --uid-owner "$DEV_UID" -p udp -d 127.0.0.1 --dport 53 -j ACCEPT
  iptables -C OUTPUT -m owner --uid-owner "$DEV_UID" -p tcp -d 127.0.0.1 --dport 53 -j ACCEPT 2>/dev/null || \
    iptables -A OUTPUT -m owner --uid-owner "$DEV_UID" -p tcp -d 127.0.0.1 --dport 53 -j ACCEPT
  iptables -C OUTPUT -m owner --uid-owner "$DEV_UID" -j REJECT 2>/dev/null || \
    iptables -A OUTPUT -m owner --uid-owner "$DEV_UID" -j REJECT
fi

# IPv6: block all egress for dev UID except ::1 (proxy/DNS)
if command -v ip6tables >/dev/null 2>&1; then
  # Allow proxy on ::1:8888 (TCP)
  ip6tables -C OUTPUT -m owner --uid-owner "$DEV_UID" -p tcp -d ::1 --dport "$PROXY_PORT" -j ACCEPT 2>/dev/null || \
    ip6tables -A OUTPUT -m owner --uid-owner "$DEV_UID" -p tcp -d ::1 --dport "$PROXY_PORT" -j ACCEPT
  # Allow local DNS on ::1:53 (TCP/UDP)
  ip6tables -C OUTPUT -m owner --uid-owner "$DEV_UID" -p udp -d ::1 --dport 53 -j ACCEPT 2>/dev/null || \
    ip6tables -A OUTPUT -m owner --uid-owner "$DEV_UID" -p udp -d ::1 --dport 53 -j ACCEPT
  ip6tables -C OUTPUT -m owner --uid-owner "$DEV_UID" -p tcp -d ::1 --dport 53 -j ACCEPT 2>/dev/null || \
    ip6tables -A OUTPUT -m owner --uid-owner "$DEV_UID" -p tcp -d ::1 --dport 53 -j ACCEPT
  # Reject all other IPv6 egress for dev UID
  ip6tables -C OUTPUT -m owner --uid-owner "$DEV_UID" -j REJECT 2>/dev/null || \
    ip6tables -A OUTPUT -m owner --uid-owner "$DEV_UID" -j REJECT
fi

# No resolv.conf changes; DNS redirection is per-UID via iptables NAT

exit 0
