#!/usr/bin/env bash
set -euo pipefail
# Build/test files and retained databases stay outside the managed checkout.
task_dir="$(mktemp -d "${TMPDIR:-/tmp}/cortex-assistant-acceptance-XXXXXX")"
trap 'rm -rf "$task_dir"' EXIT
bash scripts/go.sh build -o "$task_dir/worker" github.com/colony-2/c2j/cmd/c2j
git clone --quiet https://github.com/colony-2/cortex-assistant.git "$task_dir/assistant"
git -C "$task_dir/assistant" checkout --quiet 0561937f92a8e15c9eedb0d3045ab432f8e2c17a
bash scripts/go.sh build -o "$task_dir/host" ./server/cmd/assistanttestserver
"$task_dir/host" --worker "$task_dir/worker" --assistant-source "$task_dir/assistant"
