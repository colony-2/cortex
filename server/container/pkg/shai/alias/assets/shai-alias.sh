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
require date

hostport=${SHAI_ALIAS_SSH_HOSTPORT:-}
user=${SHAI_ALIAS_SSH_USER:-}
pass=${SHAI_ALIAS_SSH_PASS:-}
session=${SHAI_ALIAS_SESSION_ID:-}

if [ -z "$hostport" ] || [ -z "$user" ] || [ -z "$pass" ]; then
    echo "shai-alias: missing SHAI_ALIAS_* environment variables" >&2
    exit 125
fi

verbose=0
while [ $# -gt 0 ]; do
    case "$1" in
        --verbose|-v)
            verbose=1
            shift
            ;;
        --)
            shift
            break
            ;;
        *)
            break
            ;;
    esac
done

mode=list
if [ "${1:-}" = "--list" ]; then
    shift
elif [ $# -ge 1 ]; then
    mode=run
else
    echo "Usage: shai-alias [--verbose] [--list] <alias> [args...]" >&2
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

run_diagnostics() {
    diag_mode=$1
    diag_alias=$2
    diag_args=$3
    log "=== diagnostics start ==="
    log ":: date :: $(date -Iseconds)"
    log "mode=$diag_mode alias=${diag_alias:-<none>} args=${diag_args:-<none>}"
    log "--- alias env ---"
    (env | sort | grep -E '^(SHAI_ALIAS|ALLOW_DOCKER_HOST_PORT|DEV_UID)') >&2 || true
    log "ALLOW_DOCKER_HOST_PORT=${ALLOW_DOCKER_HOST_PORT:-<unset>}"
    if command -v id >/dev/null 2>&1; then
        log "id: $(id 2>/dev/null)"
    fi
    if [ -n "$port" ]; then
        if command -v nc >/dev/null 2>&1; then
            log "--- nc host.docker.internal:$port ---"
            nc -vz host.docker.internal "$port" >&2 || true
        else
            log "nc not installed; skipping connectivity test"
        fi
    else
        log "port unknown; skipping connectivity test"
    fi
    log "=== diagnostics end ==="
}

alias_name=""
extra_args=""
if [ "$mode" = "run" ]; then
    alias_name=$1
    shift
    if [ $# -gt 0 ]; then
        extra_args="$*"
    fi
fi

if [ "$verbose" -eq 1 ]; then
    run_diagnostics "$mode" "$alias_name" "$extra_args"
fi

echo "shai-alias: mode=$mode host=$host port=${port:-default} user=$user alias=${alias_name:-} args=$extra_args" >&2

if [ "$mode" = "list" ]; then
    sshpass -p "$pass" ssh $ssh_opts "$dest" -- alias-list
    exit_code=$?
    if [ $exit_code -ne 0 ]; then
        die "alias-list failed (exit $exit_code)"
    fi
    exit 0
fi

args_b64=$(printf "%s" "$extra_args" | base64 | tr -d '\n')

if ! sshpass -p "$pass" ssh $ssh_opts "$dest" -- "alias-run $alias_name $args_b64"; then
    exit_code=$?
    die "alias-run $alias_name failed (exit $exit_code)"
fi
exit 0
