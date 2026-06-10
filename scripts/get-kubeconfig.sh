#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/lib/openbao-auth.sh"

# Retrieve kubeconfig files from OpenBao into ~/.kube.
# Runs both locally and remotely (over ssh).

BAO_ADDR="${OPENBAO_ADDR:-https://openbao.home.butaco.net}"
export BAO_ADDR

if ! command -v bao &>/dev/null; then
  echo "Error: bao CLI not found" >&2
  exit 1
fi

BAO_USERNAME="${BAO_USERNAME:-homelab}"
openbao_authenticate

echo "Retrieving kubeconfig from OpenBao..."
KUBE_DIR="${HOME}/.kube"
mkdir -p "$KUBE_DIR"

dev_tmp="$(mktemp "${KUBE_DIR}/dev.yaml.tmp.XXXXXX")"
prd_tmp="$(mktemp "${KUBE_DIR}/prd.yaml.tmp.XXXXXX")"
cleanup() {
  rm -f "$dev_tmp" "$prd_tmp"
}
trap cleanup EXIT

bao kv get -field=kubeconfig secret/kubeconfig/dev > "$dev_tmp"
bao kv get -field=kubeconfig secret/kubeconfig/prd > "$prd_tmp"

for kubeconfig in "$dev_tmp" "$prd_tmp"; do
  if [[ ! -s "$kubeconfig" ]]; then
    echo "Error: OpenBao returned an empty kubeconfig." >&2
    exit 1
  fi
done

chmod 600 "$dev_tmp" "$prd_tmp"
mv "$dev_tmp" "${KUBE_DIR}/dev.yaml"
mv "$prd_tmp" "${KUBE_DIR}/prd.yaml"
trap - EXIT
echo "Kubeconfig retrieved into ~/.kube."
