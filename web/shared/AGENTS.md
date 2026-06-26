# Cortex Shared Web Package

`@colony2/shared` contains the small browser-facing API client, shared types,
auth helpers, URL helpers, and input activity SSE context used by the Cortex
React app.

This package should stay independent of the retired OpenAPI-generated client
and the removed UI packages. Keep API calls aligned with the Go Cortex server:

- `/api/projects?tenantId=...`
- `/api/projects/:projectId/cells`
- `/api/projects/:projectId/jobs`
- `/api/projects/:projectId/jobs/:jobId`
- `/api/projects/:projectId/user-inputs/...`

`projectId` values are JobDB tenant IDs in the current UI.

## Commands

Run commands from `/src`:

```bash
pnpm --filter @colony2/shared typecheck
pnpm --filter @colony2/shared test
pnpm --filter @colony2/shared build
```

The build script cleans `dist`, emits TypeScript declarations with `tsc`, then
builds the JS bundle with Vite.
