#!/usr/bin/env bash
set -euo pipefail

# Sync dotfiles installers, the mise config and lockfile, and kitty defaults.
#
# Usage:
#   sync.sh                     Fetch the latest main and overwrite the vendored copies.
#   sync.sh --check             Check vendored copies against their recorded revision.
#   sync.sh --check --upstream  Check vendored copies against the latest main.
#   REF=<sha|tag> sync.sh       Pin to a specific ref instead of main.

REPO="${REPO:-takanao14/dotfiles}"
REF_PROVIDED="${REF+x}"
REF="${REF:-main}"

# Map: <vendored filename, the chezmoi target name> -> <path within the dotfiles repo>
declare -A FILES=(
  ["10_linux_package.sh"]=".chezmoiscripts/run_onchange_10_linux_package.sh"
  ["20_linux_terminal.sh"]=".chezmoiscripts/run_onchange_20_linux_terminal.sh"
  ["30_linux_fonts.sh"]=".chezmoiscripts/run_onchange_30_linux_fonts.sh"
)
MISE_INSTALLER_SOURCE=".chezmoiscripts/run_onchange_after_40_linux_mise.sh.tmpl"
MISE_INSTALLER_DEST="40_linux_mise.sh"
MISE_CONFIG_SOURCE="dot_config/mise/config.toml"
MISE_CONFIG_DEST="mise-config.toml"
MISE_LOCK_SOURCE="dot_config/mise/mise.lock"
MISE_LOCK_DEST="mise.lock"
KITTY_CONFIG_SOURCE="dot_config/kitty/kitty.conf"

VENDOR_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REVISION_FILE="${VENDOR_DIR}/REVISION"
KITTY_CONFIG_DEST="${VENDOR_DIR}/../../../packer/files/kitty.conf"

CHECK=0
UPSTREAM=0
for arg in "$@"; do
  case "$arg" in
    --check)    CHECK=1 ;;
    --upstream) UPSTREAM=1 ;;
    *) echo "Unknown option: ${arg}" >&2; exit 1 ;;
  esac
done

# A bare --check validates the vendored copies against their own pin, so it
# never reports that the pin itself fell behind. --upstream drops that override
# and compares against REF (main by default) instead.
if [[ "$CHECK" -eq 1 && "$UPSTREAM" -eq 0 && -z "$REF_PROVIDED" && -f "$REVISION_FILE" ]]; then
  REPO="$(awk '$1 == "repo:" { print $2 }' "$REVISION_FILE")"
  REF="$(awk '$1 == "sha:" { print $2 }' "$REVISION_FILE")"
fi

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
curl -fsSL "https://raw.githubusercontent.com/${REPO}/${SHA}/${MISE_CONFIG_SOURCE}" \
  -o "${tmp_dir}/${MISE_CONFIG_DEST}"
curl -fsSL "https://raw.githubusercontent.com/${REPO}/${SHA}/${MISE_LOCK_SOURCE}" \
  -o "${tmp_dir}/${MISE_LOCK_DEST}"
curl -fsSL "https://raw.githubusercontent.com/${REPO}/${SHA}/${MISE_INSTALLER_SOURCE}" \
  -o "${tmp_dir}/${MISE_INSTALLER_DEST}.tmpl"

if command -v sha256sum >/dev/null 2>&1; then
  mise_config_sha=$(sha256sum "${tmp_dir}/${MISE_CONFIG_DEST}" | awk '{print $1}')
  mise_lock_sha=$(sha256sum "${tmp_dir}/${MISE_LOCK_DEST}" | awk '{print $1}')
else
  mise_config_sha=$(shasum -a 256 "${tmp_dir}/${MISE_CONFIG_DEST}" | awk '{print $1}')
  mise_lock_sha=$(shasum -a 256 "${tmp_dir}/${MISE_LOCK_DEST}" | awk '{print $1}')
fi
sed \
  -e "s#{{ include \"dot_config/mise/config.toml\" | sha256sum }}#${mise_config_sha}#" \
  -e "s#{{ include \"dot_config/mise/mise.lock\" | sha256sum }}#${mise_lock_sha}#" \
  "${tmp_dir}/${MISE_INSTALLER_DEST}.tmpl" > "${tmp_dir}/${MISE_INSTALLER_DEST}"

if [[ "$CHECK" -eq 1 ]]; then
  drift=0
  for name in "${!FILES[@]}"; do
    if ! diff -q "${VENDOR_DIR}/${name}" "${tmp_dir}/${name}" >/dev/null 2>&1; then
      echo "DRIFT: ${name} differs from ${REPO}@${SHA}" >&2
      drift=1
    fi
  done
  for name in "$MISE_INSTALLER_DEST" "$MISE_CONFIG_DEST" "$MISE_LOCK_DEST"; do
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
install -m 0755 "${tmp_dir}/${MISE_INSTALLER_DEST}" "${VENDOR_DIR}/${MISE_INSTALLER_DEST}"
install -m 0644 "${tmp_dir}/${MISE_CONFIG_DEST}" "${VENDOR_DIR}/${MISE_CONFIG_DEST}"
install -m 0644 "${tmp_dir}/${MISE_LOCK_DEST}" "${VENDOR_DIR}/${MISE_LOCK_DEST}"
install -m 0644 "${tmp_dir}/kitty.conf" "$KITTY_CONFIG_DEST"

cat > "$REVISION_FILE" <<EOF
# Vendored from ${REPO}, synced by sync.sh. Do not edit the installers,
# mise-config.toml, mise.lock, or packer/files/kitty.conf; re-run sync.sh to update them.
repo: ${REPO}
ref:  ${REF}
sha:  ${SHA}
date: $(date -u +%Y-%m-%dT%H:%M:%SZ)
EOF

echo "Synced vendored installers from ${REPO}@${SHA}."
