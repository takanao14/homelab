#!/usr/bin/env bash
set -euo pipefail

# Install the homelab CLI toolchain in the selected scope:
#
#   local  (default)  per-user    -> $HOME/.local/share/mise     (no sudo)
#   global            system-wide -> /usr/local/share/mise       (via sudo)
#
# Usage: tools.sh [local|global]

MODE="${1:-local}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Packer can override the separately staged vendor directory.
VENDOR_DIR="${VENDOR_DIR:-${SCRIPT_DIR}/vendor}"
INSTALLER="${VENDOR_DIR}/run_onchange_linux1_mise.sh"
VENDORED_CONFIG="${VENDOR_DIR}/mise-config.toml"

if [[ ! -f "$INSTALLER" || ! -f "$VENDORED_CONFIG" ]]; then
  echo "Error: vendored mise installer or config not found in ${VENDOR_DIR}" >&2
  echo "Run vendor/sync.sh to populate them." >&2
  exit 1
fi

case "$MODE" in
  local)
    MISE_CONFIG_FILE="${HOME}/.config/mise/config.toml"
    install -D -m 0644 "$VENDORED_CONFIG" "$MISE_CONFIG_FILE"
    RUNNER=(env
      "MISE_INSTALL_SCOPE=user"
      "MISE_CONFIG_FILE=${MISE_CONFIG_FILE}"
      bash)
    ;;
  global)
    MISE_CONFIG_FILE="/etc/mise/config.toml"
    sudo install -D -m 0644 "$VENDORED_CONFIG" "$MISE_CONFIG_FILE"
    # Preserve assignments through sudo without requiring sudoers setenv.
    RUNNER=(sudo env
      "MISE_INSTALL_SCOPE=system"
      "MISE_CONFIG_FILE=${MISE_CONFIG_FILE}"
      bash)
    ;;
  *)
    echo "Usage: $(basename "$0") [local|global]" >&2
    exit 1
    ;;
esac

"${RUNNER[@]}" "$INSTALLER"

if [[ "$MODE" == "global" ]]; then
  profile_tmp="$(mktemp)"
  trap 'rm -f "$profile_tmp"' EXIT
  {
    printf '%s\n' "export PATH=\"\$HOME/.local/share/mise/shims:/usr/local/share/mise/shims:\$PATH\""
    printf '%s\n' "export HELM_PLUGINS=\"\${HELM_PLUGINS:-/usr/local/share/helm/plugins}\""
  } >"$profile_tmp"
  sudo install -D -m 0644 "$profile_tmp" /etc/profile.d/mise.sh
fi
