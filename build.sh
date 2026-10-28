#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"

# Verification environments may have Node/pnpm but no Go bootstrap executable.
# Reuse an installed Go when available; scripts/go.sh selects the pinned version.
if ! command -v go >/dev/null 2>&1; then
  go_version="$(cat .go-version)"
  [[ "$go_version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo 'Invalid .go-version' >&2; exit 1; }
  case "$(uname -s)" in
    Linux) go_os=linux ;;
    Darwin) go_os=darwin ;;
    *) echo 'Go bootstrap supports Linux and macOS; install Go on PATH for this platform.' >&2; exit 1 ;;
  esac
  case "$(uname -m)" in
    x86_64|amd64) go_arch=amd64 ;;
    aarch64|arm64) go_arch=arm64 ;;
    *) echo 'Unsupported Go bootstrap architecture; install Go on PATH.' >&2; exit 1 ;;
  esac
  go_cache="$PWD/.tmp/toolchains"
  go_install="$go_cache/go${go_version}.${go_os}-${go_arch}"
  if [[ ! -x "$go_install/bin/go" ]]; then
    mkdir -p "$go_cache"
    go_stage="$(mktemp -d "$go_cache/install.XXXXXXXX")"
    trap 'rm -rf "$go_stage"' EXIT
    go_archive="go${go_version}.${go_os}-${go_arch}.tar.gz"
    echo "Bootstrapping $go_archive"
    curl --fail --silent --show-error --location --retry 3 \
      'https://go.dev/dl/?mode=json&include=all' -o "$go_stage/releases.json"
    go_checksum="$(node - "$go_stage/releases.json" "$go_version" "$go_archive" <<'JS'
const fs = require('node:fs');
const releases = JSON.parse(fs.readFileSync(process.argv[2], 'utf8'));
const release = releases.find(item => item.version === `go${process.argv[3]}`);
const file = release?.files.find(item => item.filename === process.argv[4]);
if (!file || !/^[a-f0-9]{64}$/.test(file.sha256)) throw new Error('Pinned Go archive or checksum unavailable');
process.stdout.write(file.sha256);
JS
)"
    curl --fail --silent --show-error --location --retry 3 \
      "https://go.dev/dl/$go_archive" -o "$go_stage/$go_archive"
    (
      cd "$go_stage"
      if command -v sha256sum >/dev/null 2>&1; then
        printf '%s  %s\n' "$go_checksum" "$go_archive" | sha256sum --check
      else
        printf '%s  %s\n' "$go_checksum" "$go_archive" | shasum -a 256 --check
      fi
      tar -xzf "$go_archive"
    )
    mv "$go_stage/go" "$go_install"
    rm -rf "$go_stage"
    trap - EXIT
  fi
  export PATH="$go_install/bin:$PATH"
fi

pnpm install --frozen-lockfile
make typecheck test build
# Browser integration checks use the existing local JobDB/worker harness:
# pnpm --filter @colony2/app test:e2e tests/goals.spec.ts tests/recipe-input.spec.ts --workers=2
