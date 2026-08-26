#!/usr/bin/env bash
set -euo pipefail

# Sync dotfiles installers and the kitty defaults used by Packer.
#
# Usage:
#   sync.sh            Fetch the latest main and overwrite the vendored copies.
#   sync.sh --check    Check vendored copies for drift.
#   REF=<sha|tag> sync.sh   Pin to a specific ref instead of main.

REPO="${REPO:-takanao14/dotfiles}"
REF="${REF:-main}"

# Map: <vendored filename> -> <path within the dotfiles repo>
declare -A FILES=(
  ["run_onchange_linux0_package.sh"]=".chezmoiscripts/run_onchange_linux0_package.sh"
  ["run_onchange_linux1_tool.sh"]=".chezmoiscripts/run_onchange_linux1_tool.sh"
  ["run_onchange_linux2_terminal.sh"]=".chezmoiscripts/run_onchange_linux2_terminal.sh"
  ["run_onchange_linux3_fonts.sh"]=".chezmoiscripts/run_onchange_linux3_fonts.sh"
)
KITTY_CONFIG_SOURCE="dot_config/kitty/kitty.conf"

VENDOR_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REVISION_FILE="${VENDOR_DIR}/REVISION"
KITTY_CONFIG_DEST="${VENDOR_DIR}/../../../packer/files/kitty.conf"

CHECK=0
[[ "${1:-}" == "--check" ]] && CHECK=1

# Resolve the ref to a concrete commit SHA so the result is reproducible.
commits_json=$(curl -fsSL "https://api.github.com/repos/${REPO}/commits/${REF}")
SHA=$(grep -m1 '"sha"' <<<"$commits_json" | grep -o '[a-f0-9]\{40\}')
if [[ -z "$SHA" ]]; then
  echo "Error: could not resolve ${REPO}@${REF} to a commit SHA" >&2
  exit 1
fi

tmp_dir="$(mktemp -d)"
cleanup() { rm -rf "$tmp_dir"; }
trap cleanup EXIT

for name in "${!FILES[@]}"; do
  curl -fsSL "https://raw.githubusercontent.com/${REPO}/${SHA}/${FILES[$name]}" \
    -o "${tmp_dir}/${name}"
done
curl -fsSL "https://raw.githubusercontent.com/${REPO}/${SHA}/${KITTY_CONFIG_SOURCE}" \
  -o "${tmp_dir}/kitty.conf"

if [[ "$CHECK" -eq 1 ]]; then
  drift=0
  for name in "${!FILES[@]}"; do
    if ! diff -q "${VENDOR_DIR}/${name}" "${tmp_dir}/${name}" >/dev/null 2>&1; then
      echo "DRIFT: ${name} differs from ${REPO}@${SHA}" >&2
      drift=1
    fi
  done
  if ! diff -q "$KITTY_CONFIG_DEST" "${tmp_dir}/kitty.conf" >/dev/null 2>&1; then
    echo "DRIFT: packer/files/kitty.conf differs from ${REPO}@${SHA}" >&2
    drift=1
  fi
  if [[ "$drift" -eq 1 ]]; then
    echo "Vendored installers are out of date. Run vendor/sync.sh to update." >&2
    exit 1
  fi
  echo "Vendored installers are in sync with ${REPO}@${SHA}."
  exit 0
fi

for name in "${!FILES[@]}"; do
  install -m 0755 "${tmp_dir}/${name}" "${VENDOR_DIR}/${name}"
done
install -m 0644 "${tmp_dir}/kitty.conf" "$KITTY_CONFIG_DEST"

cat > "$REVISION_FILE" <<EOF
# Vendored from ${REPO}, synced by sync.sh. Do not edit the run_onchange_*.sh
# files or packer/files/kitty.conf by hand; re-run sync.sh to update them.
repo: ${REPO}
ref:  ${REF}
sha:  ${SHA}
date: $(date -u +%Y-%m-%dT%H:%M:%SZ)
EOF

echo "Synced vendored installers from ${REPO}@${SHA}."
