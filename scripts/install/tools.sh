#!/usr/bin/env bash
set -euo pipefail

# Install the homelab CLI toolchain by fetching the dotfiles installer and
# running it. The install mode selects where the tools land:
#
#   local  (default)  per-user    -> $HOME/.local/bin            (no sudo)
#   global            system-wide -> /usr/local/bin              (via sudo)
#
# Use global for shared / golden-image VMs where every user needs the tools;
# use local when provisioning a personal VM for a single user.
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
    # `sudo env VAR=val` survives sudo's env reset and does not depend on the
    # sudoers `setenv` option (a bare `sudo VAR=val` can be rejected).
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

# Run the vendored copy of the dotfiles installer (see vendor/), not a fresh
# download from GitHub, so provisioning does not depend on the GitHub API rate
# limit or raw.githubusercontent.com being reachable. Refresh it with vendor/sync.sh.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# VENDOR_DIR lets callers that upload the wrapper and the vendored files to
# separate locations (e.g. the Packer shell provisioner) point at the copies.
VENDOR_DIR="${VENDOR_DIR:-${SCRIPT_DIR}/vendor}"
INSTALLER="${VENDOR_DIR}/run_onchange_linux1_tool.sh"
if [[ ! -f "$INSTALLER" ]]; then
  echo "Error: vendored installer not found: $INSTALLER" >&2
  echo "Run vendor/sync.sh to populate it." >&2
  exit 1
fi
"${RUNNER[@]}" "$INSTALLER"
