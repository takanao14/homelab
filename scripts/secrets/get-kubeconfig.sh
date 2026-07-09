#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/../lib/openbao-auth.sh"

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

# Environments to fetch kubeconfig for.
ENVS=(prd sandbox)

declare -A tmp_files
cleanup() {
  rm -f "${tmp_files[@]}"
}
trap cleanup EXIT

for env in "${ENVS[@]}"; do
  tmp="$(mktemp "${KUBE_DIR}/${env}.yaml.tmp.XXXXXX")"
  tmp_files["$env"]="$tmp"
  bao kv get -field=kubeconfig "secret/kubeconfig/${env}" > "$tmp"
  if [[ ! -s "$tmp" ]]; then
    echo "Error: OpenBao returned an empty kubeconfig for ${env}." >&2
    exit 1
  fi
done

for env in "${ENVS[@]}"; do
  tmp="${tmp_files[$env]}"
  chmod 600 "$tmp"
  mv "$tmp" "${KUBE_DIR}/${env}.yaml"
done

trap - EXIT
echo "Kubeconfig retrieved into ~/.kube."
