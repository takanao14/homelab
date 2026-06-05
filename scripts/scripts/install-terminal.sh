#!/usr/bin/env bash
set -euo pipefail

# Install the kitty terminal by fetching the dotfiles installer and running it.
# The install mode selects where kitty lands:
#
#   local  (default)  per-user    -> $HOME/.local/kitty.app      (no sudo)
#   global            system-wide -> /usr/local/kitty.app         (via sudo)
#
# Use global for shared / golden-image VMs where every user needs kitty; use
# local when provisioning a personal VM for a single user.
#
# Set TOOL_FORCE_GUI_INSTALL=1 to bypass the dotfiles installer's live-GUI
# check (needed when baking into a golden image where xrdp is not yet running,
# e.g. during a Packer build).
#
# Usage: install-terminal.sh [local|global]

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
    # `sudo env VAR=val` survives sudo's env reset and does not depend on the
    # sudoers `setenv` option (a bare `sudo VAR=val` can be rejected).
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

# Forward the GUI-check bypass when the caller requested it.
[[ "${TOOL_FORCE_GUI_INSTALL:-}" == "1" ]] && ENVS+=("TOOL_FORCE_GUI_INSTALL=1")

RUNNER=("${PRIV[@]}" "${ENVS[@]}" bash)

REPO="takanao14/dotfiles"
FILE=".chezmoiscripts/run_onchange_linux2_terminal.sh"

# Capture the API response first; piping curl directly into `grep -m1` makes
# grep close the pipe early, so curl dies with "(23) write error" under pipefail.
commits_json=$(curl -fsSL "https://api.github.com/repos/${REPO}/commits/main")
SHA=$(grep -m1 '"sha"' <<<"$commits_json" | grep -o '[a-f0-9]\{40\}')
curl -fsSL "https://raw.githubusercontent.com/${REPO}/${SHA}/${FILE}" | "${RUNNER[@]}"
