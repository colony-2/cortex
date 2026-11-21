#!/usr/bin/env bash
# Compile the general Playwright servers before its short startup timers begin.
# Each managed browser task has its own worktree and may have cold Go caches.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."
make build test-worker previous-worker
bash scripts/go.sh build -o build/cortex-ui-test-server ./server/cmd/uitestserver
