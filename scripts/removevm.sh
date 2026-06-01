#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TF_DIR="${SCRIPT_DIR}/../tf"

ENV_FILE="${HOME}/.env"
if [[ -f "$ENV_FILE" ]]; then
  set -a; source "$ENV_FILE"; set +a
fi

usage() {
  cat <<EOF
Usage: $(basename "$0") <name> [node] [--keep]

  name      VM name
  node      Proxmox node: dev | prd | node2 | node3  (default: dev)
  --keep    Keep the terragrunt directory after destroy

Example:
  $(basename "$0") myvm
  $(basename "$0") myvm prd
  $(basename "$0") myvm dev --keep
EOF
  exit 1
}

[[ $# -lt 1 ]] && usage

VM_NAME="$1"
NODE="${2:-dev}"
KEEP=false

for arg in "$@"; do
  [[ "$arg" == "--keep" ]] && KEEP=true
done

# Strip --keep from positional args for NODE resolution
if [[ "${2:-}" == "--keep" ]]; then
  NODE="dev"
fi

case "$NODE" in
  dev|prd|node2|node3) ;;
  --keep) NODE="dev" ;;
  *) echo "Error: node must be 'dev' or 'prd' or 'node2' or 'node3'" >&2; exit 1 ;;
esac

NODE_UPPER="${NODE^^}"
_node_var() { local var="${1}_${NODE_UPPER}"; echo "${!var:-}"; }

TF_VM_USERNAME="${TF_VM_USERNAME:-dummy}"
export TF_VM_USERNAME

TF_VM_SSH_PUBLIC_KEY="${TF_VM_SSH_PUBLIC_KEY:-dummy}"
export TF_VM_SSH_PUBLIC_KEY

OUT_DIR="${TF_DIR}/vm/${NODE}/${VM_NAME}"

if [[ ! -d "$OUT_DIR" ]]; then
  echo "Error: ${OUT_DIR} does not exist" >&2
  exit 1
fi

TF_VM_PASSWORD="${TF_VM_PASSWORD:-dummy}"
export TF_VM_PASSWORD

echo ""
echo "Target: tf/vm/${NODE}/${VM_NAME}"
echo "---"
cat "${OUT_DIR}/terragrunt.hcl"
echo "---"
echo ""

direnv exec "$OUT_DIR" bash -c "cd '$OUT_DIR' && terragrunt init -upgrade && terragrunt destroy"

if [[ "$KEEP" == false ]]; then
  rm -rf "$OUT_DIR"
  echo ""
  echo "Removed: tf/vm/${NODE}/${VM_NAME}"
fi
