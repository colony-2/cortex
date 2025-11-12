#!/bin/sh
set -eu

require() {
    if ! command -v "$1" >/dev/null 2>&1; then
        echo "shai-alias: missing required tool '$1'" >&2
        exit 125
    fi
}

require ssh
require sshpass
require base64

hostport=${SHAI_ALIAS_SSH_HOSTPORT:-}
user=${SHAI_ALIAS_SSH_USER:-}
pass=${SHAI_ALIAS_SSH_PASS:-}
session=${SHAI_ALIAS_SESSION_ID:-}

if [ -z "$hostport" ] || [ -z "$user" ] || [ -z "$pass" ]; then
    echo "shai-alias: missing SHAI_ALIAS_* environment variables" >&2
    exit 125
fi

mode=list
if [ "${1:-}" = "--list" ]; then
    shift
elif [ $# -ge 1 ]; then
    mode=run
else
    echo "Usage: shai-alias [--list] <alias> [args...]" >&2
    exit 2
fi

ssh_opts="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o BatchMode=yes -o ConnectTimeout=5 -o ConnectionAttempts=3 -o ServerAliveInterval=5 -o ServerAliveCountMax=2"
if [ -t 0 ]; then
    ssh_opts="$ssh_opts -tt"
fi

host=$hostport
port=""
case "$hostport" in
  *:*)
    host=${hostport%%:*}
    port=${hostport##*:}
    ;;
esac
if [ -n "$port" ] && [ "$port" != "$hostport" ]; then
    ssh_opts="$ssh_opts -p $port"
fi
dest="$user@$host"

log() {
    printf 'shai-alias: %s\n' "$*" >&2
}

die() {
    log "error: $*"
    exit 125
}

echo "shai-alias: mode=$mode host=$host port=${port:-default} user=$user alias=${1:-} args=$*" >&2

if [ "$mode" = "list" ]; then
    sshpass -p "$pass" ssh $ssh_opts "$dest" -- alias-list
    exit_code=$?
    if [ $exit_code -ne 0 ]; then
        die "alias-list failed (exit $exit_code)"
    fi
    exit 0
fi

alias_name=$1
shift
extra=""
if [ $# -gt 0 ]; then
    extra="$*"
fi

args_b64=$(printf "%s" "$extra" | base64 | tr -d '\n')

sshpass -p "$pass" ssh $ssh_opts "$dest" -- "alias-run $alias_name $args_b64"
exit_code=$?
if [ $exit_code -ne 0 ]; then
    die "alias-run $alias_name failed (exit $exit_code)"
fi
exit 0
