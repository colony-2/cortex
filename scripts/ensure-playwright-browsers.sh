#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export PLAYWRIGHT_BROWSERS_PATH="${CORTEX_PLAYWRIGHT_BROWSERS_PATH:-$HOME/.cache/ms-playwright}"
cd "$ROOT_DIR/web/app"
pnpm exec playwright install --only-shell chromium
