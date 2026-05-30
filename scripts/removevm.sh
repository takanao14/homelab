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

TF_VM_USERNAME="$(_node_var TF_VM_USERNAME)"
TF_VM_PASSWORD="$(_node_var TF_VM_PASSWORD)"

if [[ -z "$TF_VM_USERNAME" ]]; then
  TF_VM_USERNAME="$USER"
fi
export TF_VM_USERNAME

if [[ -z "${TF_VM_SSH_PUBLIC_KEY:-}" ]]; then
  DEFAULT_PUBKEY="${HOME}/.ssh/id_ed25519.pub"
  if [[ -f "$DEFAULT_PUBKEY" ]]; then
    TF_VM_SSH_PUBLIC_KEY="$(cat "$DEFAULT_PUBKEY")"
    export TF_VM_SSH_PUBLIC_KEY
  fi
fi

OUT_DIR="${TF_DIR}/vm/${NODE}/${VM_NAME}"

if [[ ! -d "$OUT_DIR" ]]; then
  echo "Error: ${OUT_DIR} does not exist" >&2
  exit 1
fi

if [[ -z "$TF_VM_PASSWORD" ]]; then
  read -rsp "VM password (${NODE}): " TF_VM_PASSWORD
  echo ""
fi
export TF_VM_PASSWORD

echo ""
echo "Target: tf/vm/${NODE}/${VM_NAME}"
echo "---"
cat "${OUT_DIR}/terragrunt.hcl"
echo "---"
echo ""

read -r -p "Destroy? (y/N) " confirm
if [[ "${confirm,,}" != "y" ]]; then
  echo "Aborted."
  exit 0
fi

direnv exec "$OUT_DIR" bash -c "cd '$OUT_DIR' && terragrunt init -upgrade && terragrunt destroy"

if [[ "$KEEP" == false ]]; then
  rm -rf "$OUT_DIR"
  echo ""
  echo "Removed: tf/vm/${NODE}/${VM_NAME}"
fi
