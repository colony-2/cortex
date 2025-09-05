# Shai Programmatic API for Cortex to Launch Nucleus

## Overview
Enable Cortex to programmatically use Shai to start the Nucleus process inside a devcontainer, exposing Shai’s core ephemeral devcontainer features via a clean, generic API. The Shai CLI must continue to work unchanged. Shai must not be customized for any specific entrypoint; the API remains entrypoint-agnostic.

## Design Goals
- Generic: support running any post-setup command, not just Nucleus.
- Minimal, composable API additions over existing `pkg/shai` types.
- Preserve CLI defaults and UX; no behavior changes to `shai` binary.
- Clean lifecycle: progress events, output streaming, cancellation, and cleanup.
- No coupling to Temporal or Nucleus types.

## Current Shai Layout (recap)
- `pkg/shai/ephemeral_runner.go`: Implements ephemeral devcontainer creation, lifecycle script generation, progress parsing, attach, and cleanup using Docker SDK directly.
- `pkg/shai/mounts.go`: Selective read-write mount builder; base workspace at `/workspace` read-only with path overlays RW.
- `pkg/shai/progress.go`: Progress phases and callbacks; `ProgressUpdate` for lifecycle markers.
- `internal/devcontainer/*`: Devcontainer parsing, Docker config build, Docker client wrapper.
- `pkg/container/*`: Public container manager interface (currently a stub outside `internal/devcontainer`). Not used by `EphemeralRunner`.

Rationale: `EphemeralRunner` is the correct integration surface for Cortex because it already encapsulates devcontainer setup, feature resolution, lifecycle script execution, progress handling, and Docker attach. We extend it (non-breaking) to allow executing an arbitrary command post-setup instead of dropping into an interactive shell.

## Proposed API Additions (pkg/shai)

### New Types
- `type ExecSpec struct {`
  `Command []string`  // argv to exec as final step (required)
  `Env map[string]string` // additional environment for the process
  `Workdir string` // container workdir; default: devcontainer workspace folder
  `UseTTY bool` // allocate TTY; default true; set false for structured logs
`}`

- `type OutputSink interface {`
  `OnStdout(data []byte)`
  `OnStderr(data []byte)`
  `// Optional: line-mode helpers may be provided by adapters in shai`
`}`

- `type Session struct {`
  `ContainerID string`
  `// Wait blocks until container exits (or context canceled)`
  `Wait(ctx context.Context) error`
  `// Stop sends SIGTERM, waits grace period, then SIGKILL` 
  `Stop(ctx context.Context) error`
  `// Close releases resources (idempotent); does not imply Stop` 
  `Close() error`
`}`

### EphemeralConfig (backward-compatible extension)
Add optional fields to `pkg/shai.EphemeralConfig`:
- `PostSetupExec *ExecSpec` — if set, run this command after devcontainer setup instead of launching an interactive login shell.
- `Output OutputSink` — optional sinks for streaming stdout/stderr from the final process. If nil and `UseTTY` is true, attaches to the current TTY (existing behavior). If nil and `UseTTY` is false, data is buffered and only printed on error.
- `GracefulStopTimeout time.Duration` — time to wait after SIGTERM before SIGKILL on Stop/cancel (default 10s).

No changes to existing fields or defaults; if `PostSetupExec` is nil, behavior is identical to today (switch to target user and `exec` login shell).

### EphemeralRunner Methods
- `func (r *EphemeralRunner) Run(ctx context.Context) error`
  - Existing method remains; if `PostSetupExec` is set, it blocks until the process exits and returns its exit status.
  - If unset, retains current behavior: attach to interactive shell, return when container exits.

- `func (r *EphemeralRunner) Start(ctx context.Context) (*Session, error)`
  - New non-blocking variant. Starts the ephemeral container and returns a `Session` handle.
  - If `PostSetupExec` is nil (interactive shell), `Start` still returns a `Session`; callers can `Wait/Stop` as needed.

### Progress and Output Semantics
- Progress: continue to emit `EphemeralProgressCallback` events reflecting devcontainer phases (FEATURES, ONCREATE, UPDATECONTENT, POSTCREATE, POSTSTART, POSTATTACH, USERSWITCH). Setting `HideProgressMarkers` controls whether raw markers are surfaced; progress callbacks always receive structured updates.
- Output: when `PostSetupExec` is set:
  - Before USERSWITCH: lifecycle output is parsed for progress; not forwarded to sinks unless `HideProgressMarkers=false`.
  - After USERSWITCH: application output (stdout/stderr) is forwarded to `OutputSink`. If `UseTTY=true`, the runner attaches to the caller’s TTY; otherwise it demuxes stdout/stderr and streams to sinks. The CLI remains unchanged and continues to attach a shell.

### Signals and Cancellation
- Context cancellation or `Session.Stop` initiate graceful termination of the container:
  - Send SIGTERM to PID 1 in the container (the setup script `exec`s the final command so it becomes PID 1).
  - Wait `GracefulStopTimeout`; if still running, send SIGKILL.
  - Cleanup temporary feature directories and close the Docker client.

### Errors and Exit Status
- If the final process exits non-zero, `Run` and `Session.Wait` return an error including the exit code. If output was buffered, a tail (configurable size) is included in the error for diagnostics.
- Setup failures (image pull, container create, lifecycle script) return explicit error contexts prior to starting the command.

## Execution Model
1. Devcontainer is loaded and variables expanded (unchanged).
2. Features are resolved and mounted (unchanged).
3. Container is created and started (unchanged).
4. Lifecycle setup script runs. Placeholders are expanded as today.
5. Final step:
   - If `PostSetupExec==nil`: `sudo -iu <user> /bin/bash -l` (current behavior).
   - If `PostSetupExec!=nil`: `sudo -iu <user> sh -lc 'exec <Command...>'` with provided `Env` and `Workdir`.
6. Attach and stream output per `UseTTY` and `OutputSink`.
7. On cancel/stop, terminate container as described above.

## Path and Working Directory Semantics
- Default container workspace folder is the devcontainer’s `workspaceFolder` (or `/workspace`).
- `ExecSpec.Workdir` defaults to this workspace folder.
- Callers should pass container-native paths in `Command`. For portability, prefer relative paths resolved under `Workdir` (e.g., `./server/nucleus/cmd/nucleus`).

## Backward Compatibility
- CLI (`cmd/shai`) remains unchanged in behavior and flags.
- New API is opt-in via `PostSetupExec` and `Start` method; existing `Run` continue to behave as today when `PostSetupExec` is nil.

## Example: Cortex Launches Nucleus via Shai

```go
import (
  "context"
  shai "github.com/divisive-ai/vibethis/server/container/pkg/shai"
)

// inside Cortex code
cfg := shai.EphemeralConfig{
  WorkingDir:     repoRoot,               // host path containing devcontainer.json
  ReadWritePaths: []string{".cache", ".vibethis"}, // overlays
  HideProgressMarkers: true,              // keep logs clean; still get structured progress
  PostSetupExec: &shai.ExecSpec{
    // Run Nucleus inside container; use relative path under workspace
    Command: []string{"go", "run", "./server/nucleus/cmd/nucleus", 
      "--name", workerName,
      "--recipes-path", "./recipes",
      "--temporal-server", temporalAddr,
      "--namespace", namespace,
    },
    Env: map[string]string{
      // optional envs if required by Nucleus or its dependencies
      "GODEBUG": "x509sha1=1",
    },
    Workdir: "",      // default workspace folder
    UseTTY:  false,    // structured logs
  },
  Output: shai.LineSink(func(stream, line string) { // helper adapter provided by shai
    // stream is "stdout" or "stderr"
    // forward to Cortex logger or event bus
  }),
}

runner, err := shai.NewEphemeralRunner(cfg)
if err != nil { /* handle */ }
defer runner.Close()

// Non-blocking start; supervise with session
sess, err := runner.Start(ctx)
if err != nil { /* handle */ }

// Optionally, wait or integrate into supervisor
go func() {
  if err := sess.Wait(ctx); err != nil { /* handle process exit */ }
}()

// On shutdown
_ = sess.Stop(shutdownCtx)
```

Notes:
- The example runs `go run` to avoid shipping a prebuilt Nucleus binary; if the container image includes the binary, replace `Command` accordingly.
- `ReadWritePaths` should include any directories where Nucleus will write (e.g., `.vibethis`, caches, temp dirs). The base workspace remains read-only for safety.

## Non-Goals
- No special-casing of Nucleus in Shai.
- No persistence APIs here (ephemeral only). If persistent containers are needed later, extend via `pkg/container.Manager`.
- No additional user-facing CLI flags at this time; CLI stays focused on interactive shells.

## Open Questions / Future Enhancements
- Output policy: add a `OutputMode` to control how much setup output vs app output is surfaced.
- Session inspection: expose `ContainerID` getters for tooling that wants Docker introspection.
- Persistent mode: mirror `EphemeralRunner` command execution into a persistent container lifecycle for long-lived workers.

