#!/usr/bin/env bash
set -euo pipefail

DEV_UID=${DEV_UID:-1000}
PROXY_PORT=${PROXY_PORT:-8888}
# Egress relaxations
# ALLOW_LOOPBACK=1 enables all loopback egress on lo (default)
# Set ALLOW_LOOPBACK=0 to disable
ALLOW_LOOPBACK=${ALLOW_LOOPBACK:-1}

if command -v iptables >/dev/null 2>&1; then
  # Allow full loopback egress if enabled
  if [ "${ALLOW_LOOPBACK}" = "1" ]; then
    iptables -C OUTPUT -m owner --uid-owner "$DEV_UID" -o lo -j ACCEPT 2>/dev/null || \
      iptables -A OUTPUT -m owner --uid-owner "$DEV_UID" -o lo -j ACCEPT
  fi
  # Allow tinyproxy access
  iptables -C OUTPUT -m owner --uid-owner "$DEV_UID" -p tcp -d 127.0.0.1 --dport "$PROXY_PORT" -j ACCEPT 2>/dev/null || \
    iptables -A OUTPUT -m owner --uid-owner "$DEV_UID" -p tcp -d 127.0.0.1 --dport "$PROXY_PORT" -j ACCEPT
  # Reject all other egress for dev UID
  iptables -C OUTPUT -m owner --uid-owner "$DEV_UID" -j REJECT 2>/dev/null || \
    iptables -A OUTPUT -m owner --uid-owner "$DEV_UID" -j REJECT
fi

## IPv6 rules intentionally omitted
