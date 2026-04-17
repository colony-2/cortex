#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TEST_RUNTIME_DIR="${TEST_RUNTIME_DIR:-$ROOT_DIR/.tmp/run-all-tests}"
DEFAULT_PLAYWRIGHT_BROWSERS_PATH="/tmp/ms-playwright"
PLAYWRIGHT_BROWSERS_PATH="${PLAYWRIGHT_BROWSERS_PATH:-$DEFAULT_PLAYWRIGHT_BROWSERS_PATH}"

mkdir -p \
  "$TEST_RUNTIME_DIR/tmp" \
  "$TEST_RUNTIME_DIR/go-build" \
  "$TEST_RUNTIME_DIR/go-cache"

export TMPDIR="$TEST_RUNTIME_DIR/tmp"
export GOTMPDIR="$TEST_RUNTIME_DIR/go-build"
export GOCACHE="$TEST_RUNTIME_DIR/go-cache"

log() {
  printf '\n==> %s\n' "$1"
}

run() {
  local description="$1"
  shift
  log "$description"
  "$@"
}

has_playwright_chromium() {
  local browser_path="$1"

  [ -d "$browser_path" ] || return 1
  find "$browser_path" -maxdepth 1 \
    \( -name 'chromium-*' -o -name 'chromium_headless_shell-*' \) \
    -print -quit | grep -q .
}

if ! has_playwright_chromium "$PLAYWRIGHT_BROWSERS_PATH" && \
  has_playwright_chromium "$DEFAULT_PLAYWRIGHT_BROWSERS_PATH"; then
  PLAYWRIGHT_BROWSERS_PATH="$DEFAULT_PLAYWRIGHT_BROWSERS_PATH"
fi

ensure_playwright_chromium() {
  if has_playwright_chromium "$PLAYWRIGHT_BROWSERS_PATH"; then
    return
  fi

  run "Installing Playwright Chromium" \
    bash -lc "cd '$ROOT_DIR/server/cortex' && PLAYWRIGHT_BROWSERS_PATH='$PLAYWRIGHT_BROWSERS_PATH' pnpm exec playwright install chromium"
}

run_go_workspace_tests() {
  while read -r module; do
    [ -n "$module" ] || continue
    run "Go tests in $module" bash -lc "cd '$ROOT_DIR/$module' && go test ./..."
  done < <(awk '/^[[:space:]]+\.\// { gsub(/^[[:space:]]+\.\//, "", $1); print $1 }' "$ROOT_DIR/go.work")
}

run_web_tests() {
  run "Vitest in web/shared" bash -lc "cd '$ROOT_DIR/web/shared' && pnpm exec vitest run"
  run "Vitest in web/kanban" bash -lc "cd '$ROOT_DIR/web/kanban' && pnpm exec vitest run"
  run "Vitest in web/flowchart" bash -lc "cd '$ROOT_DIR/web/flowchart' && pnpm exec vitest run"
  run "Vitest in web/openapi" bash -lc "cd '$ROOT_DIR/web/openapi' && pnpm exec vitest run"
  run "Vitest in web/app" bash -lc "cd '$ROOT_DIR/web/app' && npm test -- --run"

  if find "$ROOT_DIR/web/notebook" \
    \( -path '*/node_modules/*' -o -path '*/dist/*' \) -prune -o \
    \( -type f \( -name '*.test.*' -o -name '*.spec.*' \) -print -quit \) | grep -q .; then
    run "Vitest in web/notebook" bash -lc "cd '$ROOT_DIR/web/notebook' && pnpm exec vitest run"
  else
    log "Skipping web/notebook (no test files)"
  fi

  ensure_playwright_chromium
  run "Playwright in server/cortex" \
    bash -lc "cd '$ROOT_DIR/server/cortex' && PLAYWRIGHT_BROWSERS_PATH='$PLAYWRIGHT_BROWSERS_PATH' pnpm test"
  run "Playwright in web/app" \
    bash -lc "cd '$ROOT_DIR/web/app' && PLAYWRIGHT_BROWSERS_PATH='$PLAYWRIGHT_BROWSERS_PATH' npm run test:e2e"
}

run_go_workspace_tests
run_web_tests
