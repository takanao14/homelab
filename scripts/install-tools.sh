#!/usr/bin/env bash
set -euo pipefail

# Tool installation is managed in dotfiles.
# This script downloads and executes the canonical installer from there.
SCRIPT_URL="https://raw.githubusercontent.com/takanao14/dotfiles/main/.chezmoiscripts/run_onchange_linux1_tool.sh"

curl -fsSL "$SCRIPT_URL" | bash
