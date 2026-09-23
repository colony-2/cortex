#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# c2j's current dependencies require Go 1.26. Allow explicit caller overrides.
export GOTOOLCHAIN="${GOTOOLCHAIN:-go$(cat "$ROOT_DIR/.go-version")}"
exec go "$@"
