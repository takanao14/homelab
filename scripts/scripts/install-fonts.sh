#!/usr/bin/env bash
set -euo pipefail

# Install the UDEV Gothic NF font by fetching the dotfiles installer and
# running it. The install mode selects where the font lands:
#
#   local  (default)  per-user    -> $HOME/.local/share/fonts   (no sudo)
#   global            system-wide -> /usr/local/share/fonts      (via sudo)
#
# Use global for shared / golden-image VMs where every user needs the font;
# use local when provisioning a personal VM for a single user.
#
# Set TOOL_FORCE_GUI_INSTALL=1 to bypass the dotfiles installer's live-GUI
# check (needed when baking into a golden image where xrdp is not yet running,
# e.g. during a Packer build).
#
# Usage: install-fonts.sh [local|global]

MODE="${1:-local}"

case "$MODE" in
  local)
    FONT_DIR="${HOME}/.local/share/fonts"
    CACHE_DIR="${HOME}/.local/share/tool-versions"
    PRIV=(env)
    ;;
  global)
    # `sudo env VAR=val` survives sudo's env reset and does not depend on the
    # sudoers `setenv` option (a bare `sudo VAR=val` can be rejected).
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
# Forward the GUI-check bypass when the caller requested it.
[[ "${TOOL_FORCE_GUI_INSTALL:-}" == "1" ]] && ENVS+=("TOOL_FORCE_GUI_INSTALL=1")

RUNNER=("${PRIV[@]}" "${ENVS[@]}" bash)

REPO="takanao14/dotfiles"
FILE=".chezmoiscripts/run_onchange_linux3_fonts.sh"

# Capture the API response first; piping curl directly into `grep -m1` makes
# grep close the pipe early, so curl dies with "(23) write error" under pipefail.
commits_json=$(curl -fsSL "https://api.github.com/repos/${REPO}/commits/main")
SHA=$(grep -m1 '"sha"' <<<"$commits_json" | grep -o '[a-f0-9]\{40\}')
curl -fsSL "https://raw.githubusercontent.com/${REPO}/${SHA}/${FILE}" | "${RUNNER[@]}"
