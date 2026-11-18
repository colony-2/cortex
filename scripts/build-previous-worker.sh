#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUTPUT="${1:-$ROOT_DIR/build/c2j-previous-worker}"
mkdir -p "$(dirname "$OUTPUT")"
OUTPUT="$(cd "$(dirname "$OUTPUT")" && pwd)/$(basename "$OUTPUT")"
MODULE_DIR="$(mktemp -d)"
trap 'rm -rf "$MODULE_DIR"' EXIT
# Build the actual previous worker and its own dependency graph. Linking the
# previous c2j against the upgraded JobDB would not represent the old worker.
cat > "$MODULE_DIR/go.mod" <<'EOF'
module cortex-previous-worker

go 1.26

require github.com/colony-2/c2j v0.0.129-0.20261118001543-b9d8dae96cb3
require github.com/colony-2/jobdb v0.0.89-0.20261117003434-d58b8d5c4e3b
EOF
GOWORK=off bash "$ROOT_DIR/scripts/go.sh" -C "$MODULE_DIR" build -mod=mod -o "$OUTPUT" github.com/colony-2/c2j/cmd/c2j
