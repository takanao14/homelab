#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/../../lib/openbao-auth.sh"

# Store kubeconfig files from ~/.kube in OpenBao.
# Runs both locally and remotely (over ssh).

BAO_ADDR="${OPENBAO_ADDR:-https://openbao.home.butaco.net}"
export BAO_ADDR

if ! command -v bao &>/dev/null; then
  echo "Error: bao CLI not found" >&2
  exit 1
fi

KUBE_DIR="${HOME}/.kube"
clusters=(dev prd sandbox)

for cluster in "${clusters[@]}"; do
  kubeconfig="${KUBE_DIR}/${cluster}.yaml"
  if [[ ! -s "$kubeconfig" ]]; then
    echo "Error: ${kubeconfig} not found or empty." >&2
    exit 1
  fi
done

BAO_USERNAME="${BAO_USERNAME:-admin}"
openbao_authenticate

echo "Writing kubeconfig to OpenBao..."
for cluster in "${clusters[@]}"; do
  bao kv put "secret/kubeconfig/${cluster}" "kubeconfig=@${KUBE_DIR}/${cluster}.yaml"
  echo "  secret/kubeconfig/${cluster}"
done

echo "Done."
