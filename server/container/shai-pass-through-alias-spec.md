# Shai Pass-Through Alias Commands Requirements

## Context
`shai` currently lets developers drop into a devcontainer with selective mounts, but many workflows still require invoking host-only tooling (e.g., credential helpers, macOS-only CLIs, access to local secrets). Today the only option is to exit the container or maintain two copies of tooling. This doc specifies a "pass-through alias command" system where container users run lightweight aliases whose execution transparently occurs on the host that launched `shai`.

## Goals
- Allow container users to run predefined aliases that execute host commands while staying inside the container shell.
- Provide a declarative manifest (`.shai-cmds`) so repo owners decide which host commands are exposed and how arguments may vary.
- Stream the host command's stdout/stderr/exit code back to the container, preserving ordering and allowing interactive flows (stdin + Ctrl+C).
- Support multiple concurrent `shai` sessions; each session must isolate alias execution traffic.
- Work on macOS and Linux hosts without relying on platform-specific IPC features.

## Non-Goals
- Running arbitrary host commands from inside the container (must be explicitly allowed in `.shai-cmds`).
- Providing a general-purpose remote shell; only aliased commands are supported.
- Persisting alias execution history or telemetry beyond what shai already logs.

## Terminology
- **Host**: Machine running `shai` CLI/library.
- **Container**: Devcontainer started by `shai`.
- **Alias**: User-friendly command name exposed inside the container (e.g., `shai-alias cache-clear`).
- **Backing command**: Actual host-side command/argv executed when an alias runs.
- **Alias Bridge**: Embedded `gliderlabs/ssh` server plus container helper that coordinate pass-through execution.

## `.shai-cmds` Manifest
- Lives beside `.devcontainer/devcontainer.json`. When missing, alias execution stays disabled.
- Plain text, whitespace-delimited columns: `alias  args-regex|-  command...`.
- Rules:
  - Column 1 (`alias`): unique per repo, lowercase letters/digits/`-`/`_`.
  - Column 2 (`args-regex`): RE2 pattern evaluated against the *entire* extra-args string exactly as typed in the container (after we join the argv tail with single spaces). Use `-` to disallow any extra args. Regex is anchored automatically; treat it as full match.
  - Column 3 (`command`): remainder of the line, executed on the host using the parent shell (`$SHELL` or `/bin/bash -lc`). Relative paths resolve from the repo root containing the manifest.
- Blank lines or those starting with `#` are ignored.
- Example:
  ```
  # alias   allowed-args         host command
  git-sync  -                    git pull --rebase
  apply     ^(--env=(dev|prod))$ ./scripts/apply.sh
  cache-rm  ^(--key=[a-z0-9_-]+)$ ./ops/cache_rm.sh
  ```
- Validation performed before container launch; invalid regexes or commands outside the repo root abort `shai` startup.

## Runtime Architecture
### Host Side (shai CLI/lib)
1. Parse `.shai-cmds`, compile regexes, build alias table.
2. Pick an unused loopback port per shai invocation (e.g., `localhost:48xxx`).
3. Launch an **alias supervisor subprocess** before starting the SSH bridge. Instead of shipping a new binary, shai re-execs itself with an internal flag (e.g., `SHAI_ALIAS_SUPERVISOR=1`). Parent and supervisor communicate via a control pipe; if the parent dies (including `kill -9`), the pipe closes and the supervisor immediately begins teardown.
4. Generate a single-use strong password; export `SHAI_ALIAS_SSH_HOSTPORT` (e.g., `host.docker.internal:49821`), `SHAI_ALIAS_SSH_PASS`, fixed `SHAI_ALIAS_SSH_USER=shai`, and `SHAI_ALIAS_SESSION_ID` into the container environment.
5. Start an embedded SSH server (via `gliderlabs/ssh`) bound to loopback that accepts only password auth for the generated credentials. All approved alias execs are forwarded to the supervisor over an internal RPC channel so the supervisor becomes the parent of every subprocess.
6. Bridge (SSH handler) responsibilities per session:
   - Reject non-command or interactive shell requests; only accept `exec` with `shai-alias <alias> ...`.
   - Validate alias existence and regex compliance (host always re-reads `.shai-cmds` on each request or when the file’s mtime changes so edits take effect immediately).
   - Forward approved executions to the alias supervisor, which spawns the backing command using `exec.CommandContext(shellPath, "-lc", resolvedCmd)` so execution happens in a fork of the host shell that launched `shai` while keeping the supervisor as the direct parent.
   - Wire SSH stdin/stdout/stderr directly to the spawned command so output order and control characters stay intact.
   - Forward exit codes with `session.Exit(n)` and propagate termination signals (SIGINT/SIGTERM → process, SIGKILL on timeout).
   - Enforce per-alias timeouts and session-level concurrency limits.
   - Tear down server automatically when the shai session exits.

### Container Side Entry Command
- Provide a lightweight helper (`shai-alias`) or shell function that wraps `ssh` (no custom protocol required).
- Usage: `shai-alias <alias> [extra args…]`.
- Responsibilities:
  - Read `SHAI_ALIAS_SSH_HOSTPORT`, `SHAI_ALIAS_SSH_USER`, `SHAI_ALIAS_SSH_PASS`.
  - `SHAI_ALIAS_SSH_HOSTPORT` already encodes the reachable hostname (macOS: `host.docker.internal`, Linux: injected host-gateway), so helpers do not guess the host.
  - Invoke system `ssh` with `-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null` and feed the password via `SSH_ASKPASS`/`sshpass`-style helper owned by `shai-alias` (no user prompts).
  - Forward stdin to the SSH session and stream stdout/stderr back to the container terminal verbatim.
  - Exit with the SSH session’s remote status; emit clear errors (exit 125) on connection/auth failures.

- `.shai-cmds` Path Masking: during container creation, shai bind-mounts `/dev/null` (or an empty tmpfs) over the manifest location inside the workspace so container users cannot read the host’s policy file. Only the host-side shai process reads the real file.

- Each `shai` invocation provisions a dedicated loopback port/host combination and single-use password; never reuse across sessions.
- SSH server binds to `127.0.0.1` only. On teardown, listener shuts down and password is discarded. The exported `SHAI_ALIAS_SSH_HOSTPORT` encodes the matching host alias (`host.docker.internal` vs `127.0.0.1` etc.) to keep client logic trivial.
- Container reaches the server via Docker’s host networking shim; no additional proxies required.
- Helper retries transient TCP/SSH failures (up to ~3 attempts within 2s) before surfacing errors.
- We can later swap password auth for short-lived CA-signed certificates if multi-user hardening is needed.

## Execution Semantics
- Command assembly: the supervisor constructs a single shell line as `<command-from-column3> <extra-args-string>` (extra string omitted when empty) and runs it via `exec.CommandContext(shellPath, "-lc", ...)` in the repo root.
- Additional args: helper joins remaining argv into a single string (preserving spaces quoted by user) and forwards it verbatim; the host validates this string against the column 2 regex (or `-`) before launching the backing command. On failure, the host returns exit 2 plus a friendly error message to the container.
- STDIN: streamed byte-for-byte over SSH; if a future manifest flag disables stdin for an alias, the supervisor immediately closes the stdin pipe before launching the command.
- STDOUT/STDERR: since SSH channels already carry separate stdout/stderr, we simply wire them directly to the container terminal—no custom framing or buffering beyond what OpenSSH already provides.
- Exit Codes: helper exits with the remote process exit code or 130 when interrupted (matching bash conventions).
- Logging: bridge logs start/stop events with alias name, duration, exit status, optional truncated output for debugging. Include whether the manifest reloaded between invocations and whether the supervisor was healthy.
- Interrupts: Ctrl+C in the container closes the SSH session (SIGINT); gliderlabs delivers this to the handler, which forwards SIGINT/SIGTERM to the spawned host process and escalates to SIGKILL after the configured grace period.
- Process groups: supervisor sets `SysProcAttr{Setpgid: true}` on each alias so it can deliver SIGTERM/SIGKILL to the entire subtree when the control pipe closes, the context cancels, or shai exits unexpectedly.

## Error Handling
- Manifest errors → `shai` CLI exits before container launch.
- Missing alias invocation → helper prints "unknown alias" and suggests `shai-alias --list`.
- Network/auth failures → helper prints actionable message and exit 125.
- Host command failure exit codes/STDERR simply bubble back; container user handles as normal.
- If bridge dies mid-command, helper exits 125 and instructs user to restart shai.

## Security Considerations
- `.shai-cmds` should be committed and code-reviewed; treat as execution policy.
- Bridge refuses to run commands referencing paths outside repo root; command lines are executed relative to the repo root to prevent directory traversal.
- The real `.shai-cmds` stays hidden inside the container via a bind mount to `/dev/null`, preventing users from reverse-engineering available host commands.
- Password generated per session; never log the secret (log first/last 4 chars max). Host key can also be regenerated per session to avoid persistence.
- Alias supervisor guarantees cleanup: all alias processes belong to the supervisor’s process group, so when the parent dies (even via `kill -9`) the supervisor notices the control-pipe EOF, sends SIGTERM, waits a grace period, then SIGKILLs the entire group before exiting.
- Consider future support for signed manifests (out of scope now).

## Observability & Tooling
- `shai-alias --list` reads manifest info exposed via env var and prints available aliases/usage.
- Bridge exposes debug logs under `--debug` flag; integrate with existing shai progress UI.
- Add metrics hooks (duration, success/failure) for later ingestion.

## Testing Requirements
- Unit tests for manifest parsing + regex validation.
- Integration tests using loopback docker container that runs `shai-alias` against a fake bridge to ensure streaming/interrupts.
- Cross-platform tests verifying host discovery logic (macOS vs Linux) via mocked environment.
- Load test: spawn concurrent aliases to ensure isolation and proper log streaming.

## `shai-alias` Helper
- **Form factor**: POSIX shell (`/bin/bash`) script installed into the container at `/usr/local/bin/shai-alias`. Using Bash keeps the distribution simple (no additional binaries) and lets us rely on the system `ssh` client.
- **Behavior**:
  1. Validates that required env vars (`SHAI_ALIAS_SSH_HOSTPORT`, `SHAI_ALIAS_SSH_USER`, `SHAI_ALIAS_SSH_PASS`) exist; otherwise exits 125 with guidance.
  2. Extracts the alias (`$1`) and shifts remaining args. If no alias provided, prints usage and optional `--list` output (which it obtains by asking the host for the latest alias list over SSH).
  3. Joins remaining args into a single string (`extra="$*"`). No quoting modifications are made; spaces are preserved exactly.
  4. Invokes `ssh "$SHAI_ALIAS_SSH_USER@$SHAI_ALIAS_SSH_HOSTPORT"` with `StrictHostKeyChecking=no` and a tiny `SSH_ASKPASS` helper (or `batchmode` + `sshpass`) that feeds the password. Command sent to SSH is `run-alias <alias> -- <extra-args-string>`.
  5. Streams stdin/stdout/stderr directly between the container terminal and SSH session. Exit code mirrors the remote command (except 125 for helper-level failures).
  6. Supports `--list` by executing the host subcommand `list-aliases`, which returns the latest parsed manifest. Because the host owns the manifest, alias additions/removals become visible instantly without container restarts.

## Hot Reload & Visibility
- Shai watches `.shai-cmds` using fsnotify (fallback: stat polling). When the file changes, regexes/commands are re-parsed atomically so new or updated aliases are immediately available to running containers.
- Since the container view of `.shai-cmds` is masked, the only way to discover available commands is via `shai-alias --list` (which queries the host) or internal documentation.

## Open Questions
1. Should `.shai-cmds` support templated arguments (e.g., capture groups from regex) to insert into backing command, or do we only append raw user args?
2. Do we need per-alias resource constraints (CPU/memory) enforced on host?
3. How do we distribute the `shai-alias` script when `shai` runs against arbitrary devcontainer images? (Likely via bind-mounting `/usr/local/bin/shai-alias` from host.)
4. Do we need analytics indicating alias popularity to help repo owners prune unused entries?

## OSS Framework
We standardize on **gliderlabs/ssh** as the embedded server. It delivers built-in TTY forwarding, stdin/stdout streaming, and signal propagation with minimal glue code, and it already depends on `golang.org/x/crypto/ssh`. The only extra work is handling ephemeral host keys/passwords and ensuring the container image has an `ssh` client (or bundling one in `shai-alias`).
