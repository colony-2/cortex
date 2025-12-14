VibeThis Workspace (Nx)
=======================

Primary commands (run at repo root):
- Install dev deps: `npm install`
- Project graph: `npx nx graph --file nx-graph.html`
- Build everything: `npx nx run-many --target=build --all`
- Test everything: `npx nx run-many --target=test --all`
- Single project examples: `npx nx build be-api`, `npx nx test ui-app`, `npx nx run ui-app:e2e`
- Integration examples (uncached/opt-in): `npx nx run server-llm:integration`, `npx nx run rwshim-go:integration`
- Docker builds (needs Docker): `npx nx run shai-docker:build`, `npx nx run rwshim-clib:build`

Notes:
- Go builds use the Nx-Go executors; `build` runs `go build` and `test` runs `go test` per package.
- Frontends use Nx Vite executors; `build` emits to each package `dist`.
- Nx caching is enabled; reruns reuse results unless inputs change.
