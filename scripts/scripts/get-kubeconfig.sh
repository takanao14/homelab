#!/usr/bin/env bash
set -euo pipefail

# Retrieve kubeconfig files from OpenBao into ~/.kube.
# Runs both locally and remotely (over ssh). The OpenBao password is taken from
# the BAO_PASSWORD env var, an interactive prompt (TTY), or stdin (non-interactive).

BAO_ADDR="${OPENBAO_ADDR:-https://openbao.home.butaco.net}"
export BAO_ADDR

if ! command -v bao &>/dev/null; then
  echo "Error: bao CLI not found" >&2
  exit 1
fi

BAO_USERNAME="${BAO_USERNAME:-homelab}"
# Resolve password: env var > interactive prompt (TTY) > stdin (non-interactive)
if [[ -n "${BAO_PASSWORD:-}" ]]; then
  _bao_pass="$BAO_PASSWORD"
elif [[ -t 0 ]]; then
  read -rsp "OpenBao password for ${BAO_USERNAME}: " _bao_pass; echo
else
  read -r _bao_pass
fi
BAO_TOKEN=$(bao login -token-only -method=userpass username="${BAO_USERNAME}" password="${_bao_pass}")
export BAO_TOKEN

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
chmod 600 "$dev_tmp" "$prd_tmp"
mv "$dev_tmp" "${KUBE_DIR}/dev.yaml"
mv "$prd_tmp" "${KUBE_DIR}/prd.yaml"
trap - EXIT
echo "Kubeconfig retrieved into ~/.kube."
