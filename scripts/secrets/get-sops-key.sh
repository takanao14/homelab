#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/../lib/openbao-auth.sh"

# Retrieve the SOPS age private key from OpenBao into the local keys file.
# Runs both locally and remotely (over ssh).

BAO_ADDR="${OPENBAO_ADDR:-https://openbao.home.butaco.net}"
export BAO_ADDR

if ! command -v bao &>/dev/null; then
  echo "Error: bao CLI not found" >&2
  exit 1
fi

KEY_FILE="${SOPS_AGE_KEY_FILE:-${HOME}/.config/sops/age/keys.txt}"
KEY_DIR="$(dirname "$KEY_FILE")"

BAO_USERNAME="${BAO_USERNAME:-homelab}"
openbao_authenticate

echo "Retrieving SOPS age key from OpenBao..."
mkdir -p "$KEY_DIR"

key_tmp="$(mktemp "${KEY_FILE}.tmp.XXXXXX")"
cleanup() {
  rm -f "$key_tmp"
}
trap cleanup EXIT

bao kv get -field=keys secret/sops/age > "$key_tmp"

if [[ ! -s "$key_tmp" ]]; then
  echo "Error: OpenBao returned an empty SOPS age key." >&2
  exit 1
fi
if ! grep -q "AGE-SECRET-KEY-" "$key_tmp"; then
  echo "Error: OpenBao value does not look like an age private key." >&2
  exit 1
fi

chmod 600 "$key_tmp"
mv "$key_tmp" "$KEY_FILE"
trap - EXIT
echo "SOPS age key retrieved into ${KEY_FILE}."
