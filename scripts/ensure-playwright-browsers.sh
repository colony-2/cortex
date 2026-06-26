#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
APP_DIR="$ROOT_DIR/web/app"
BROWSER_PATH="${CORTEX_PLAYWRIGHT_BROWSERS_PATH:-$HOME/.cache/ms-playwright}"

export PLAYWRIGHT_BROWSERS_PATH="$BROWSER_PATH"

require_command() {
  local name="$1"

  if ! command -v "$name" >/dev/null 2>&1; then
    printf 'Required command not found: %s\n' "$name" >&2
    exit 1
  fi
}

first_match() {
  local pattern="$1"

  sed -nE "$pattern" | head -n 1
}

install_browser() {
  local browser="$1"
  local executable="$2"
  local dry_run
  local install_location
  local tmp_dir
  local zip_path

  dry_run="$(cd "$APP_DIR" && pnpm exec playwright install --dry-run "$browser")"
  install_location="$(printf '%s\n' "$dry_run" | first_match 's/^[[:space:]]*Install location:[[:space:]]*//p')"

  if [ -z "$install_location" ]; then
    printf 'Could not determine Playwright install location for %s\n' "$browser" >&2
    exit 1
  fi

  case "$install_location" in
    "$BROWSER_PATH"/*) ;;
    *)
      printf 'Refusing to install %s outside PLAYWRIGHT_BROWSERS_PATH: %s\n' "$browser" "$install_location" >&2
      exit 1
      ;;
  esac

  if [ -x "$install_location/$executable" ]; then
    return
  fi

  mapfile -t urls < <(
    printf '%s\n' "$dry_run" |
      sed -nE 's/^[[:space:]]*Download (url|fallback [0-9]+):[[:space:]]*//p'
  )

  if [ "${#urls[@]}" -eq 0 ]; then
    printf 'Could not determine Playwright download URL for %s\n' "$browser" >&2
    exit 1
  fi

  tmp_dir="$(mktemp -d)"
  zip_path="$tmp_dir/$browser.zip"
  trap 'rm -rf "$tmp_dir"' RETURN

  for url in "${urls[@]}"; do
    if curl --fail --location --silent --show-error --output "$zip_path" "$url"; then
      break
    fi
  done

  if [ ! -s "$zip_path" ]; then
    printf 'Failed to download Playwright artifact for %s\n' "$browser" >&2
    exit 1
  fi

  rm -rf "$install_location"
  mkdir -p "$install_location"
  unzip -q "$zip_path" -d "$install_location"

  if [ ! -f "$install_location/$executable" ]; then
    printf 'Playwright artifact for %s did not contain %s\n' "$browser" "$executable" >&2
    exit 1
  fi

  chmod 755 "$install_location/$executable"
  touch "$install_location/INSTALLATION_COMPLETE"
}

require_command curl
require_command pnpm
require_command unzip

install_browser chromium-headless-shell chrome-linux/headless_shell
install_browser ffmpeg ffmpeg-linux
