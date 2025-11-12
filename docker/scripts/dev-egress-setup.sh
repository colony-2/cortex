#!/usr/bin/env bash
set -euo pipefail

DEV_UID=${DEV_UID:-1000}
PROXY_PORT=${PROXY_PORT:-8888}
DNS_PORT=${DNS_PORT:-53}
ALLOW_DOCKER_HOST_PORT=${ALLOW_DOCKER_HOST_PORT:-}

ensure_rule() {
  local table=$1
  shift
  if ! iptables -t "$table" -C "$@" 2>/dev/null; then
    iptables -t "$table" -A "$@"
  fi
}

ensure_rule6() {
  local table=$1
  shift
  if ! ip6tables -t "$table" -C "$@" 2>/dev/null; then
    ip6tables -t "$table" -A "$@"
  fi
}

if command -v iptables >/dev/null 2>&1; then
  # Allow full loopback egress for dev UID
  ensure_rule filter OUTPUT -m owner --uid-owner "$DEV_UID" -o lo -j ACCEPT

  # Allow tinyproxy access
  ensure_rule filter OUTPUT -m owner --uid-owner "$DEV_UID" -p tcp -d 127.0.0.1 --dport "$PROXY_PORT" -j ACCEPT

  # Force DNS through the local resolver
  ensure_rule nat OUTPUT -m owner --uid-owner "$DEV_UID" -p udp --dport "$DNS_PORT" -j REDIRECT --to-ports "$DNS_PORT"
  ensure_rule nat OUTPUT -m owner --uid-owner "$DEV_UID" -p tcp --dport "$DNS_PORT" -j REDIRECT --to-ports "$DNS_PORT"

  # Allow DNS responses to localhost
  ensure_rule filter OUTPUT -m owner --uid-owner "$DEV_UID" -p udp -d 127.0.0.1 --dport "$DNS_PORT" -j ACCEPT
  ensure_rule filter OUTPUT -m owner --uid-owner "$DEV_UID" -p tcp -d 127.0.0.1 --dport "$DNS_PORT" -j ACCEPT

  # Reject all other egress for dev UID
  if [ -n "$ALLOW_DOCKER_HOST_PORT" ]; then
    echo "[dev-egress] allowing egress on port $ALLOW_DOCKER_HOST_PORT"
    ensure_rule filter OUTPUT -m owner --uid-owner "$DEV_UID" -p tcp --dport "$ALLOW_DOCKER_HOST_PORT" -j ACCEPT
  fi
  ensure_rule filter OUTPUT -m owner --uid-owner "$DEV_UID" -j REJECT
fi

if command -v ip6tables >/dev/null 2>&1; then
  ensure_rule6 filter OUTPUT -m owner --uid-owner "$DEV_UID" -p tcp -d ::1 --dport "$PROXY_PORT" -j ACCEPT
  ensure_rule6 filter OUTPUT -m owner --uid-owner "$DEV_UID" -p udp -d ::1 --dport "$DNS_PORT" -j ACCEPT
  ensure_rule6 filter OUTPUT -m owner --uid-owner "$DEV_UID" -p tcp -d ::1 --dport "$DNS_PORT" -j ACCEPT
  ensure_rule6 filter OUTPUT -m owner --uid-owner "$DEV_UID" -j REJECT
fi
