# Repository Guidelines

## Project Structure & Module Organization
- `cmd/shai/`: CLI entrypoint for the devcontainer runner (builds the `shai` binary).
- `pkg/shai/`: Ephemeral runner, mount builder, progress reporting, and related logic.
- `internal/devcontainer/`: Devcontainer parsing, Docker client/manager, and integration helpers.
- `pkg/container/`: Public interfaces and shared types.
- `spec/` and `*.md`: Architecture and behavior specs used to guide implementation.

## Build, Test, and Development Commands
- Build CLI: `go build -o shai ./cmd/shai`
- Run tests (all): `go test ./... -v`
- Run package tests: `go test ./internal/devcontainer -v`
- Coverage (quick): `go test ./... -cover`

Example: run the CLI with read‑write mounts
```
./shai -rw server/container
```

## Coding Style & Naming Conventions
- Go version: 1.24+ recommended.
- Formatting: `gofmt`/`goimports` (run `go fmt ./...`).
- Linting (suggested): `go vet ./...` before opening a PR.
- Naming: Exported identifiers use MixedCaps (Go convention). Tests live in `*_test.go` and use `TestXxx` functions.

## Testing Guidelines
- Frameworks: Go `testing`; assertions via `stretchr/testify` (assert/require) in many tests.
- Unit tests should focus on one behavior; integration tests may require Docker.
- Run a specific package: `go test ./pkg/shai -v`.
- Add tests next to the code they exercise; prefer table‑driven tests.

## Commit & Pull Request Guidelines
- Commits: Use short, imperative messages (e.g., "Fix progress renderer"), group related changes.
- PRs: Include a concise description, rationale, and testing notes. Link issues where applicable. For UI/CLI tweaks, include before/after snippets.
- Ensure: `go build ./...` and `go test ./...` pass locally before requesting review.

## Security & Configuration Tips
- Docker is required for integration paths; ensure the daemon is reachable.
- Optional: `GHCR_TOKEN`/`GITHUB_TOKEN` can reduce rate‑limits when resolving features.
- TERM is propagated to the container; use a real TTY for best interactive behavior.

