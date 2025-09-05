# VibeThis Dev Image — Quick Guide

## Overview
- Base: Debian bookworm-slim.
- Languages/Tools: Go 1.24, Rust (stable via rustup), Python 3, Node v22.9.0 (+ yarn, pnpm), C/C++ toolchain, Java (default-jdk), git, jq.
- AI CLIs (npm): `openai`, `@google/gemini-cli`, `claude-code`.
- Shells: `zsh` default for root and `devuser` (uid 1000) with Oh My Zsh.
- Proxy: tinyproxy bound to `127.0.0.1:8888`, supervised; allowlist-based egress.
- Network guard: iptables restricts `devuser` to `127.0.0.1:8888` (requires `NET_ADMIN`).
- WORKDIR: `/src`.

## Build Targets
- Dev (split logic, faster iteration): `docker build -t debian-dev:dev --target dev .`
- Prod (collapsed steps): `docker build -t debian-dev:prod --target prod .`

## Run Modes
- Default (ENTRYPOINT supervisord): `docker run -d --cap-add NET_ADMIN --name devbox debian-dev:dev`
- Root shell (bootstrap auto-starts proxy + rules):
  - `docker run --rm -it --cap-add NET_ADMIN --entrypoint /bin/zsh debian-dev:dev`
  - or `--entrypoint /bin/bash`
- Overriding to non-shell commands will NOT auto-bootstrap; chain it if needed:
  - `--entrypoint /bin/sh -lc 'bootstrap-proxy.sh && exec <cmd>'`

## Tinyproxy + Allowlist
- Config: `/etc/tinyproxy/tinyproxy.conf` (Listen 127.0.0.1; Allow 127.0.0.1; LogFile /var/log/tinyproxy/tinyproxy.log).
- Allowlist: `/etc/shai/allowed_domains.conf` (default includes OpenAI, Anthropic, Gemini, package registries, GitHub Packages, container registries, docs).
- Auto-reload on changes via inotify watcher under supervisord.
- Logs: `/var/log/tinyproxy/*.log`.

## Egress Control (devuser)
- Enforced by `/usr/local/sbin/dev-egress-setup.sh` using iptables.
- Applies at container start under supervisord and on root interactive shells via `/usr/local/sbin/bootstrap-proxy.sh`.
- Requires: `--cap-add NET_ADMIN` when running the container.

## Node/Package Managers
- Node installed from official tarball (ARG `NODE_VERSION`, pinned to 22.9.0).
- After Node extract, runs a single global install: `npm@latest`, `yarn`, `pnpm`.
- AI CLIs installed via a single `npm -g install`.

## Layering Rules (keep rebuilds fast)
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
- Check status: `supervisorctl status`.
- Logs: `/var/log/supervisor/supervisord.log`, `/var/log/tinyproxy/*.log`.
- Verify rules: `iptables -S OUTPUT | grep owner`.

## Limitations
- ENTRYPOINT overrides to non-shell processes bypass bootstrap; either chain `bootstrap-proxy.sh` or enforce egress at the Docker network/host level.
- Tinyproxy listens only on localhost (by design); not exposed to host.
