#!/usr/bin/env bash
# Fast contract gate used by the managed verify hook and local repair loop.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
node scripts/verify-command.mjs bash scripts/go.sh test -count=1 ./server/internal/cortex -run '^TestWorkspaceDiff'
pnpm --filter @colony2/shared exec vitest run src/services/reviewChangesService.test.ts
pnpm --filter @colony2/app exec vitest run src/components/ReviewChanges.test.tsx src/components/review/viewers.test.tsx
