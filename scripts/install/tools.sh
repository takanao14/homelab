#!/usr/bin/env bash
set -euo pipefail

# Install the homelab CLI toolchain in the selected scope:
#
#   local  (default)  per-user    -> $HOME/.local/bin            (no sudo)
#   global            system-wide -> /usr/local/bin              (via sudo)
#
# Usage: tools.sh [local|global]

MODE="${1:-local}"

case "$MODE" in
  local)
    RUNNER=(env
      "TOOL_BIN_DIR=${HOME}/.local/bin"
      "TOOL_VERSION_CACHE_DIR=${HOME}/.local/share/tool-versions"
      bash)
    ;;
  global)
    # Preserve assignments through sudo without requiring sudoers setenv.
    RUNNER=(sudo env
      "TOOL_BIN_DIR=/usr/local/bin"
      "TOOL_VERSION_CACHE_DIR=/usr/local/share/tool-versions"
      bash)
    ;;
  *)
    echo "Usage: $(basename "$0") [local|global]" >&2
    exit 1
    ;;
esac

# Use the vendored installer; refresh it with vendor/sync.sh.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Packer can override the separately staged vendor directory.
VENDOR_DIR="${VENDOR_DIR:-${SCRIPT_DIR}/vendor}"
INSTALLER="${VENDOR_DIR}/run_onchange_linux1_tool.sh"
if [[ ! -f "$INSTALLER" ]]; then
  echo "Error: vendored installer not found: $INSTALLER" >&2
  echo "Run vendor/sync.sh to populate it." >&2
  exit 1
fi
"${RUNNER[@]}" "$INSTALLER"
