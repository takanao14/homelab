#!/usr/bin/env bash
set -euo pipefail

REPO="takanao14/dotfiles"
FILE=".chezmoiscripts/run_onchange_linux3_fonts.sh"

SHA=$(curl -fsSL "https://api.github.com/repos/${REPO}/commits/main" \
      | grep -m1 '"sha"' | grep -o '[a-f0-9]\{40\}')
curl -fsSL "https://raw.githubusercontent.com/${REPO}/${SHA}/${FILE}" | bash
