#!/usr/bin/env bash
# Nix Chromium includes Fontconfig libraries, but needs explicit font discovery
# when the worker is not a NixOS desktop. Keep setup local to each browser task.
set -euo pipefail

font_root="$(nix build --no-link --print-out-paths github:NixOS/nixpkgs/2ad48bb4ab952d317de4a966aa0af5822c5d3d34#dejavu_fonts)"
[[ "$font_root" =~ ^/nix/store/[a-z0-9]{32}-[A-Za-z0-9.+_-]+$ ]]
[[ -d "$font_root/share/fonts" ]]
[[ -n "$(find "$font_root/share/fonts" -name '*.ttf' -print -quit)" ]]

font_scratch="$(mktemp -d /tmp/cortex-verify-fonts.XXXXXXXX)"
trap 'rm -rf "$font_scratch"' EXIT
export FONTCONFIG_FILE="$font_scratch/fonts.conf"
unset FONTCONFIG_PATH
printf '<fontconfig><dir>%s/share/fonts</dir><cachedir>%s/cache</cachedir></fontconfig>\n' \
  "$font_root" "$font_scratch" > "$FONTCONFIG_FILE"

"$@"
