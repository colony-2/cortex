# VibeThis Dev Image — Quick Guide

## Overview
- Base: Debian bookworm-slim.
- Languages/Tools: Go 1.24, Rust (stable via rustup), Python 3, Node v22.9.0 (+ yarn, pnpm), C/C++ toolchain, Java (default-jdk), git, jq.
 - Languages/Tools: Go 1.24, Rust (stable via rustup), Python 3, Node v22.9.0 (+ yarn, pnpm), C/C++ toolchain, Java (default-jdk), git, jq.
 - Browsers/Automation: Playwright CLI with Chromium preinstalled.
- AI CLIs (npm): `openai`, `@google/gemini-cli`, `claude-code`, `@moonrepo/cli`.
- Shells: `zsh` default for root and `devuser` (uid 1000) with Oh My Zsh.
- Proxy: tinyproxy bound to `127.0.0.1:8888`, supervised; allowlist-based egress.
- Network guard: iptables restricts `devuser` egress to tinyproxy and loopback (requires `NET_ADMIN`).
- WORKDIR: `/src`.

## Build Targets
- Dev (split logic, faster iteration): `docker build -t debian-dev:dev --target dev .`
- Prod (collapsed steps): `docker build -t debian-dev:prod --target prod .`
- Parity requirement: Dev and Prod targets must remain functionally identical (same tools, versions, configs). Only layering differs. Any change to installs or configuration in one target must be mirrored in the other.

## Run Modes
- Default (ENTRYPOINT supervisord): `docker run -d --cap-add NET_ADMIN --name devbox debian-dev:dev`
- Root shell: `docker run --rm -it --cap-add NET_ADMIN --entrypoint /bin/zsh debian-dev:dev`
- Overriding to non-shell commands will NOT auto-bootstrap; chain it if needed:
  - `--entrypoint /bin/sh -lc 'bootstrap.sh && exec <cmd>'`

## Tinyproxy + Allowlist
- Config: `/etc/tinyproxy/tinyproxy.conf` (Listen 127.0.0.1; Allow 127.0.0.1; LogFile `/var/log/tinyproxy/tinyproxy.log`).
- Allowlist: `/etc/shai/allowed_domains.conf` (default includes OpenAI, Anthropic, Gemini, package registries, GitHub Packages, container registries, docs).
- Auto-reload on changes via allowlist watcher under supervisord:
  - Uses inotify when available and polls as a fallback, to handle bind mounts.
  - Script: `/usr/local/sbin/allowlist-watcher.sh`.
- Logs: `/var/log/tinyproxy/*.log`.

## Egress Control (devuser)
- Enforced by `/usr/local/sbin/bootstrap.sh` and `scripts/dev-egress-setup.sh` using iptables.
- Applies at container start under supervisord and on root interactive shells via `/usr/local/sbin/bootstrap.sh`.
- Requires: `--cap-add NET_ADMIN` when running the container.
- Defaults:
  - HTTP(S) egress only via tinyproxy on `127.0.0.1:$PROXY_PORT` (default 8888).
  - DNS forced to local resolver and allowed.
  - All loopback egress allowed for `devuser` (interface `lo`).
  - All other egress REJECTed for `devuser`.
- Config (env vars):
  - `PROXY_PORT` (default `8888`): tinyproxy port to allow.
  - `ALLOW_LOOPBACK` (default `1`): allow all loopback egress when `1`; set to `0` to disable.
- Rule order: loopback rule is appended before the final REJECT; verify with `iptables -S OUTPUT | grep -E "owner| -o lo |dport"`.

Notes:
- Loopback allow means processes can talk to services on `127.0.0.1:<any>` inside the container (e.g., dev servers).
- This does not permit outbound traffic to the internet without going through the proxy; non-loopback egress remains blocked.
- Host-to-container published ports are unaffected by these OUTPUT rules.

## Node/Package Managers
- Node installed from official tarball (ARG `NODE_VERSION`, pinned to 22.9.0).
- After Node extract, runs a single global install: `npm@latest`, `yarn`, `pnpm`.
- AI CLIs installed via a single `npm -g install`.
- Playwright: globally installed as `playwright`; Chromium browser is preinstalled into `$PLAYWRIGHT_BROWSERS_PATH`.
  - `PLAYWRIGHT_BROWSERS_PATH=/ms-playwright`
  - Example: `playwright --version` and `node -e "require('playwright'); console.log('ok')"`.
  - Use headless mode by default; add `--headless=new` or rely on Playwright defaults.

## Workspace Mounting Pattern
- Mount the host workspace read-only at `/src`.
- For each writable path `rw` given to shai, mount that host subpath at the corresponding `/src/<subpath>` with `:rw`.
- Result: `/src` tree is read-only except the specific subpaths you mounted as `:rw` (unless you explicitly set `./` as a writable path, which makes `/src` itself writable).
- No `/workspace` directory is used. `/src` inside the image is root-owned and not writable by `devuser` unless you mount a `:rw` path over it.

Example (manual Docker):
- `-v "$PWD:/src:ro" -v "$PWD/subdir:/src/subdir:rw" -v "$HOME/.cache:/src/.cache:rw"`

Notes:
- Do not mount `/src` as `:rw` unless you intend to allow full-tree writes (equivalent to passing `./` as `rw`).
- This pattern requires no extra overlay scripts in the container.
- Keep “installs” early; put “configuration” (users, shells, supervisord files, proxy rules, allowlist) as late layers.
- Collapse package manager calls:
  - apt: single `apt-get update` + single `apt-get install` in one RUN.
  - npm: group global installs in one command.
- Do not introduce extra apt update/install steps unless strictly necessary.

## Change Rules
- AI CLI installs: via npm only (no pipx for non-Python tools).
- If changing proxy port or bind, update all of: tinyproxy.conf, allowlist watcher, bootstrap script, dev-egress-setup (iptables), devuser shell env.
- If adding domains/registries, edit `/etc/shai/allowed_domains.conf` (baked default comes from `shai-allowed-domains.conf`).
- If bumping Node, ensure `npm@latest` engine compatibility (or pin npm instead).
- Preserve `devuser` uid 1000 and `WORKDIR /src`.

## Supervision & Diagnostics
- Process manager: supervisord (ENTRYPOINT for default runs; daemonized by bootstrap in shell runs).
- Check status: `supervisorctl status` (requires control socket; otherwise check processes/ports).
- Logs:
  - tinyproxy internal: `/var/log/tinyproxy/tinyproxy.log` (written by tinyproxy).
  - supervisord capture: `/var/log/tinyproxy/tinyproxy.out.log` (stdout), `/var/log/tinyproxy/tinyproxy.err.log` (stderr).
  - Supervisor core: `/var/log/supervisor/supervisord.log`.
- Verify rules: `iptables -S OUTPUT | grep owner`.

## Limitations
- ENTRYPOINT overrides to non-shell processes bypass bootstrap; either chain `bootstrap.sh` or enforce egress at the Docker network/host level.
- Tinyproxy listens only on localhost (by design); not exposed to host.

## Tests
- Integration script: `tests/integration/test_image.sh`
  - Builds the dev target, runs the container with NET_ADMIN, bootstraps services, and verifies:
    - Toolchains on PATH for devuser (`go`, `cargo`)
    - DNS allow/deny from `/etc/shai/allowed_domains.conf`
    - HTTP via proxy works; direct bypass is blocked
    - Auto-reload when the allowlist is updated
- CI: GitHub Actions workflow at `.github/workflows/ci.yml` runs the integration test.

## Moonrepo
- Project file: `moon.yml` (in this directory)
- Tasks:
  - `build`: builds the Docker dev image (`docker build -t debian-dev:dev --target dev .`).
  - `integration`: runs `tests/integration/test_image.sh` (depends on `build`).
- If you override ENTRYPOINT, run `bootstrap.sh` as a postcreate step (no other scripts are required). The workspace mount pattern above is handled entirely by how you invoke Docker/shai.
