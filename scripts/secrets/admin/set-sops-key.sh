#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/../../lib/openbao-auth.sh"

# Store the local SOPS age private key in OpenBao at secret/sops/age.

BAO_ADDR="${OPENBAO_ADDR:-https://openbao.home.butaco.net}"
export BAO_ADDR

if ! command -v bao &>/dev/null; then
  echo "Error: bao CLI not found" >&2
  exit 1
fi

KEY_FILE="${SOPS_AGE_KEY_FILE:-${HOME}/.config/sops/age/keys.txt}"

if [[ ! -s "$KEY_FILE" ]]; then
  echo "Error: ${KEY_FILE} not found or empty." >&2
  exit 1
fi
if ! grep -q "AGE-SECRET-KEY-" "$KEY_FILE"; then
  echo "Error: ${KEY_FILE} does not contain an age private key." >&2
  exit 1
fi

BAO_USERNAME="${BAO_USERNAME:-admin}"
openbao_authenticate

echo "Writing SOPS age key to OpenBao..."
bao kv put secret/sops/age "keys=@${KEY_FILE}"
echo "  secret/sops/age"

echo "Done."
