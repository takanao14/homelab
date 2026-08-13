#!/usr/bin/env bash
set -euo pipefail

# Install UDEV Gothic NF in the selected scope:
#
#   local  (default)  per-user    -> $HOME/.local/share/fonts   (no sudo)
#   global            system-wide -> /usr/local/share/fonts      (via sudo)
#
# TOOL_FORCE_GUI_INSTALL=1 bypasses the live-GUI check during image builds.
#
# Usage: fonts.sh [local|global]

MODE="${1:-local}"

case "$MODE" in
  local)
    FONT_DIR="${HOME}/.local/share/fonts"
    CACHE_DIR="${HOME}/.local/share/tool-versions"
    PRIV=(env)
    ;;
  global)
    # Preserve assignments through sudo without requiring sudoers setenv.
    FONT_DIR="/usr/local/share/fonts"
    CACHE_DIR="/usr/local/share/tool-versions"
    PRIV=(sudo env)
    ;;
  *)
    echo "Usage: $(basename "$0") [local|global]" >&2
    exit 1
    ;;
esac

ENVS=(
  "TOOL_FONT_DIR=${FONT_DIR}"
  "TOOL_VERSION_CACHE_DIR=${CACHE_DIR}"
)
# Forward the optional GUI-check bypass.
[[ "${TOOL_FORCE_GUI_INSTALL:-}" == "1" ]] && ENVS+=("TOOL_FORCE_GUI_INSTALL=1")

RUNNER=("${PRIV[@]}" "${ENVS[@]}" bash)

# Use the vendored installer; refresh it with vendor/sync.sh.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Packer can override the separately staged vendor directory.
VENDOR_DIR="${VENDOR_DIR:-${SCRIPT_DIR}/vendor}"
INSTALLER="${VENDOR_DIR}/run_onchange_linux3_fonts.sh"
if [[ ! -f "$INSTALLER" ]]; then
  echo "Error: vendored installer not found: $INSTALLER" >&2
  echo "Run vendor/sync.sh to populate it." >&2
  exit 1
fi
"${RUNNER[@]}" "$INSTALLER"
