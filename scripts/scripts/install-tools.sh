#!/usr/bin/env bash
set -euo pipefail

REPO="takanao14/dotfiles"
FILE=".chezmoiscripts/run_onchange_linux1_tool.sh"

# Capture the API response first; piping curl directly into `grep -m1` makes
# grep close the pipe early, so curl dies with "(23) write error" under pipefail.
commits_json=$(curl -fsSL "https://api.github.com/repos/${REPO}/commits/main")
SHA=$(grep -m1 '"sha"' <<<"$commits_json" | grep -o '[a-f0-9]\{40\}')
curl -fsSL "https://raw.githubusercontent.com/${REPO}/${SHA}/${FILE}" | bash
