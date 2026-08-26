#!/usr/bin/env bash
set -euo pipefail

# Install kitty in the selected scope:
#
#   local  (default)  per-user    -> $HOME/.local/kitty.app      (no sudo)
#   global            system-wide -> /usr/local/kitty.app         (via sudo)
#
# TOOL_MACHINE_PROFILE=desktop|server selects desktop components explicitly.
# TOOL_FORCE_GUI_INSTALL=1 remains as a deprecated compatibility override.
#
# Usage: terminal.sh [local|global]

MODE="${1:-local}"

case "$MODE" in
  local)
    ENVS=(
      "TOOL_BIN_DIR=${HOME}/.local/bin"
      "TOOL_KITTY_PREFIX=${HOME}/.local"
      "TOOL_APPS_DIR=${HOME}/.local/share/applications"
      "TOOL_VERSION_CACHE_DIR=${HOME}/.local/share/tool-versions"
    )
    PRIV=(env)
    ;;
  global)
    # Preserve assignments through sudo without requiring sudoers setenv.
    ENVS=(
      "TOOL_BIN_DIR=/usr/local/bin"
      "TOOL_KITTY_PREFIX=/usr/local"
      "TOOL_APPS_DIR=/usr/local/share/applications"
      "TOOL_VERSION_CACHE_DIR=/usr/local/share/tool-versions"
    )
    PRIV=(sudo env)
    ;;
  *)
    echo "Usage: $(basename "$0") [local|global]" >&2
    exit 1
    ;;
esac

# Forward the machine profile and legacy GUI-check bypass.
[[ -v TOOL_MACHINE_PROFILE ]] && ENVS+=("TOOL_MACHINE_PROFILE=${TOOL_MACHINE_PROFILE}")
[[ "${TOOL_FORCE_GUI_INSTALL:-}" == "1" ]] && ENVS+=("TOOL_FORCE_GUI_INSTALL=1")

RUNNER=("${PRIV[@]}" "${ENVS[@]}" bash)

# Use the vendored installer; refresh it with vendor/sync.sh.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Packer can override the separately staged vendor directory.
VENDOR_DIR="${VENDOR_DIR:-${SCRIPT_DIR}/vendor}"
INSTALLER="${VENDOR_DIR}/run_onchange_linux2_terminal.sh"
if [[ ! -f "$INSTALLER" ]]; then
  echo "Error: vendored installer not found: $INSTALLER" >&2
  echo "Run vendor/sync.sh to populate it." >&2
  exit 1
fi
"${RUNNER[@]}" "$INSTALLER"
