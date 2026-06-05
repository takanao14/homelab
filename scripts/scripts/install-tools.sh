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
# Usage: install-tools.sh [local|global]

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

REPO="takanao14/dotfiles"
FILE=".chezmoiscripts/run_onchange_linux1_tool.sh"

# Capture the API response first; piping curl directly into `grep -m1` makes
# grep close the pipe early, so curl dies with "(23) write error" under pipefail.
commits_json=$(curl -fsSL "https://api.github.com/repos/${REPO}/commits/main")
SHA=$(grep -m1 '"sha"' <<<"$commits_json" | grep -o '[a-f0-9]\{40\}')
curl -fsSL "https://raw.githubusercontent.com/${REPO}/${SHA}/${FILE}" | "${RUNNER[@]}"
