#!/usr/bin/env bash
set -euo pipefail

# Install CLI prerequisites and select the version-cache scope:
#
#   local  (default)  per-user    -> $HOME/.local/share/tool-versions
#   global            system-wide -> /usr/local/share/tool-versions (via sudo)
#
# TOOL_SKIP_SYSTEM_PACKAGES=1 performs a no-sudo prerequisite check.
#
# Usage: packages.sh [local|global]

MODE="${1:-local}"

case "$MODE" in
  local)
    ENVS=(
      "TOOL_VERSION_CACHE_DIR=${HOME}/.local/share/tool-versions"
    )
    PRIV=(env)
    ;;
  global)
    # Preserve assignments through sudo without requiring sudoers setenv.
    ENVS=(
      "TOOL_VERSION_CACHE_DIR=/usr/local/share/tool-versions"
    )
    PRIV=(sudo env)
    ;;
  *)
    echo "Usage: $(basename "$0") [local|global]" >&2
    exit 1
    ;;
esac

for name in TOOL_SKIP_SYSTEM_PACKAGES TOOL_MACHINE_PROFILE TOOL_FORCE_GUI_INSTALL \
            KUBECTL_VERSION OPENBAO_VERSION FREELENS_VERSION; do
  [[ -v "$name" ]] && ENVS+=("${name}=${!name}")
done

RUNNER=("${PRIV[@]}" "${ENVS[@]}" bash)

# Use the vendored installer; refresh it with vendor/sync.sh.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Packer can override the separately staged vendor directory.
VENDOR_DIR="${VENDOR_DIR:-${SCRIPT_DIR}/vendor}"
INSTALLER="${VENDOR_DIR}/run_onchange_linux0_package.sh"
if [[ ! -f "$INSTALLER" ]]; then
  echo "Error: vendored installer not found: $INSTALLER" >&2
  echo "Run vendor/sync.sh to populate it." >&2
  exit 1
fi
"${RUNNER[@]}" "$INSTALLER"
