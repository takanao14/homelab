#!/usr/bin/env bash
set -euo pipefail

# Install the system packages required by the homelab CLI toolchain. The install
# mode selects where version-cache markers land:
#
#   local  (default)  per-user    -> $HOME/.local/share/tool-versions
#   global            system-wide -> /usr/local/share/tool-versions (via sudo)
#
# Set TOOL_SKIP_SYSTEM_PACKAGES=1 for a no-sudo preflight that verifies the
# packages were already provided, for example by a golden-image build.
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
    # `sudo env VAR=val` survives sudo's env reset and does not depend on the
    # sudoers `setenv` option (a bare `sudo VAR=val` can be rejected).
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

for name in TOOL_SKIP_SYSTEM_PACKAGES KUBECTL_VERSION OPENBAO_VERSION; do
  [[ -v "$name" ]] && ENVS+=("${name}=${!name}")
done

RUNNER=("${PRIV[@]}" "${ENVS[@]}" bash)

# Run the vendored copy of the dotfiles installer (see vendor/), not a fresh
# download from GitHub, so provisioning does not depend on the GitHub API rate
# limit or raw.githubusercontent.com being reachable. Refresh it with vendor/sync.sh.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# VENDOR_DIR lets callers that upload the wrapper and the vendored files to
# separate locations (e.g. the Packer shell provisioner) point at the copies.
VENDOR_DIR="${VENDOR_DIR:-${SCRIPT_DIR}/vendor}"
INSTALLER="${VENDOR_DIR}/run_onchange_linux0_package.sh"
if [[ ! -f "$INSTALLER" ]]; then
  echo "Error: vendored installer not found: $INSTALLER" >&2
  echo "Run vendor/sync.sh to populate it." >&2
  exit 1
fi
"${RUNNER[@]}" "$INSTALLER"
