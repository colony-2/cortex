#!/usr/bin/env bash
set -euo pipefail

DEV_UID=${DEV_UID:-1000}
PROXY_PORT=${PROXY_PORT:-8888}
DNS_PORT=${DNS_PORT:-53}
ALLOW_DOCKER_HOST_PORT=${ALLOW_DOCKER_HOST_PORT:-}
DOCKER_HOST_NAME=${DOCKER_HOST_NAME:-}

# Prefer an explicit host name if provided, otherwise infer from alias hostport, else default.
if [ -z "$DOCKER_HOST_NAME" ] && [ -n "${SHAI_ALIAS_SSH_HOSTPORT:-}" ]; then
  # Handle values like host:port or [ipv6]:port; strip brackets and port.
  host_part=${SHAI_ALIAS_SSH_HOSTPORT%%:*}
  host_part=${host_part#[}
  host_part=${host_part%]}
  DOCKER_HOST_NAME=$host_part
fi
DOCKER_HOST_NAME=${DOCKER_HOST_NAME:-host.docker.internal}

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

resolve_host_ip() {
  local name=$1
  local ip=""
  local ping_output=""

  if command -v ping >/dev/null 2>&1; then
    if ping_output=$(ping -c1 -W1 "$name" 2>/dev/null); then
      ip=$(printf '%s' "$ping_output" | sed -n '1s/.*(\(.*\)).*/\1/p')
    fi
  fi

  if [ -z "$ip" ] && command -v getent >/dev/null 2>&1; then
    ip=$(getent hosts "$name" | awk '{print $1; exit}')
  fi

  echo "$ip"
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
    host_ip=$(resolve_host_ip "$DOCKER_HOST_NAME")
    if [ -n "$host_ip" ]; then
      echo "[dev-egress] allowing egress on port $ALLOW_DOCKER_HOST_PORT to $DOCKER_HOST_NAME ($host_ip)"
      ensure_rule filter OUTPUT -m owner --uid-owner "$DEV_UID" -p tcp -d "$host_ip" --dport "$ALLOW_DOCKER_HOST_PORT" -j ACCEPT
    else
      echo "[dev-egress] WARNING: unable to resolve $DOCKER_HOST_NAME, allowing port $ALLOW_DOCKER_HOST_PORT without destination restriction"
      ensure_rule filter OUTPUT -m owner --uid-owner "$DEV_UID" -p tcp --dport "$ALLOW_DOCKER_HOST_PORT" -j ACCEPT
    fi
  fi
  ensure_rule filter OUTPUT -m owner --uid-owner "$DEV_UID" -j REJECT

  echo "[dev-egress] iptables OUTPUT rules:"
  iptables -S OUTPUT || true
fi

if command -v ip6tables >/dev/null 2>&1; then
  ensure_rule6 filter OUTPUT -m owner --uid-owner "$DEV_UID" -p tcp -d ::1 --dport "$PROXY_PORT" -j ACCEPT
  ensure_rule6 filter OUTPUT -m owner --uid-owner "$DEV_UID" -p udp -d ::1 --dport "$DNS_PORT" -j ACCEPT
  ensure_rule6 filter OUTPUT -m owner --uid-owner "$DEV_UID" -p tcp -d ::1 --dport "$DNS_PORT" -j ACCEPT
  ensure_rule6 filter OUTPUT -m owner --uid-owner "$DEV_UID" -j REJECT

  echo "[dev-egress] ip6tables OUTPUT rules:"
  ip6tables -S OUTPUT || true
fi
