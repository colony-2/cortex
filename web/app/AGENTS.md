# Cortex Web App

This package is the React UI for Cortex. It is intentionally scoped to the
new decomponentized model:

- projects in routes are JobDB tenant IDs, defaulting to the tenant in the configured JobDB URI
- jobs are the primary work item
- cells are the current cell plus c2j-discovered dependents
- pending inputs are job input requests

Do not reintroduce removed legacy work-item screens, recipe management screens,
project editing/admin pages, graph traversal UI, old task-runner commands, or
the retired auxiliary UI packages.

## Commands

Run commands from `/src`:

```bash
pnpm --filter @colony2/app dev
pnpm --filter @colony2/app typecheck
pnpm --filter @colony2/app test
pnpm --filter @colony2/app build
```

The production build is bundled into the Go `cortex` executable by the root
`Makefile`.

## Routing

The app reads its initial tenant from `/api/projects`, using the server’s configured
JobDB URI. A URL tenant overrides that default. A different tenant can be selected
through the header control or by passing `?tenantId=<id>`.

Primary routes:

- `/project/:projectId/jobs`
- `/project/:projectId/jobs/:jobId/story`
- `/project/:projectId/inputs`
- `/project/:projectId/cells`

Use `@colony2/shared` for API calls and input activity state.
