#!/usr/bin/env bash
set -euo pipefail

DEV_UID=${DEV_UID:-1000}

if command -v iptables >/dev/null 2>&1; then
  # Allow only connections to local tinyproxy (127.0.0.1:8888) for dev UID
  iptables -C OUTPUT -m owner --uid-owner "$DEV_UID" -p tcp -d 127.0.0.1 --dport 8888 -j ACCEPT 2>/dev/null || \
    iptables -A OUTPUT -m owner --uid-owner "$DEV_UID" -p tcp -d 127.0.0.1 --dport 8888 -j ACCEPT
  # Reject all other egress for dev UID
  iptables -C OUTPUT -m owner --uid-owner "$DEV_UID" -j REJECT 2>/dev/null || \
    iptables -A OUTPUT -m owner --uid-owner "$DEV_UID" -j REJECT
fi

