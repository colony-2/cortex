#!/usr/bin/env bash
set -euo pipefail

IMG_TAG=${IMG_TAG:-debian-dev:dev}
ALLOWED=${ALLOWED:-api.openai.com}
BLOCKED=${BLOCKED:-example.org}

cleanup() {
  docker rm -f devbox-test >/dev/null 2>&1 || true
}
trap cleanup EXIT

# NOTE: This script assumes the image is already built.
# Moon's `integration` task depends on `build`, so do not rebuild here.

echo "[run] container"
docker run -d --rm --cap-add NET_ADMIN --name devbox-test \
  -v "$PWD/shai-allowed-domains.conf:/etc/shai/allowed_domains.conf:rw" \
  "$IMG_TAG" >/dev/null

echo "[bootstrap] start services + iptables"
docker exec devbox-test /usr/local/sbin/bootstrap.sh

echo "[check] toolchains on path for devuser"
docker exec -u devuser devbox-test sh -lc 'command -v go && go version && command -v cargo && cargo --version' >/dev/null

echo "[check] dns allow resolves"
for attempt in $(seq 1 5); do
  if docker exec -u devuser devbox-test sh -lc "getent hosts $ALLOWED" >/dev/null; then
    break
  fi
  if [ "$attempt" -eq 5 ]; then
    echo "failed to resolve $ALLOWED within startup window" >&2
    exit 1
  fi
  sleep 1
done
echo "[check] dns block fails"
if docker exec -u devuser devbox-test sh -lc "getent hosts $BLOCKED"; then
  echo "expected DNS block for $BLOCKED" >&2; exit 1; fi

echo "[check] http allowed via proxy"
for attempt in $(seq 1 5); do
  if docker exec -u devuser devbox-test sh -lc "curl -I -m 10 https://$ALLOWED" >/dev/null; then
    break
  fi
  if [ "$attempt" -eq 5 ]; then
    echo "failed to reach $ALLOWED via proxy" >&2
    exit 1
  fi
  sleep 1
done

echo "[check] http bypass blocked"
if docker exec -u devuser devbox-test sh -lc "curl --noproxy '*' -4 -I -m 5 https://$ALLOWED"; then
  echo "expected http bypass to be blocked" >&2; exit 1; fi

echo "[check] TTY echo and prompt readiness"
# Verify stty reports echo enabled in an interactive root zsh session
docker exec -t devbox-test zsh -ic 'stty -a' | tr -s ' ' | grep -q ' echo ' || { echo "stty echo not enabled" >&2; exit 1; }

echo "[check] /workspace not present and /src not writable by devuser"
# No /workspace directory should exist
if docker exec devbox-test sh -lc '[ -d /workspace ]'; then echo "/workspace should not exist" >&2; exit 1; fi
# devuser cannot write to /src by default
if docker exec -u devuser devbox-test sh -lc 'echo test >/src/.writetest'; then echo "/src should not be writable by devuser" >&2; exit 1; fi

echo "ok"
