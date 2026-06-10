#!/usr/bin/env bash
set -euo pipefail

# Destroy a VM created by create-vm.sh and remove its Terragrunt directory
# (kept when --keep is given).

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TF_DIR="${SCRIPT_DIR}/../tf"

ENV_FILE="${HOME}/.env"
if [[ -f "$ENV_FILE" ]]; then
  set -a
  # shellcheck source=/dev/null
  source "$ENV_FILE"
  set +a
fi

usage() {
  cat <<EOF
Usage: $(basename "$0") <name> [node] [--keep]

  name      VM name
  node      Proxmox node: dev | node2 | node3 (default: dev)
  --keep    Keep the terragrunt directory after destroy

Example:
  $(basename "$0") myvm
  $(basename "$0") myvm node2
  $(basename "$0") myvm dev --keep
EOF
  exit 1
}

[[ $# -lt 1 || $# -gt 3 ]] && usage

VM_NAME="$1"
NODE="dev"
KEEP=false
NODE_SET=false

if [[ ! "$VM_NAME" =~ ^[a-zA-Z0-9-]+$ ]]; then
  echo "Error: VM name must contain only alphanumeric characters and hyphens" >&2
  exit 1
fi

shift
for arg in "$@"; do
  case "$arg" in
    --keep)
      KEEP=true
      ;;
    dev|node2|node3)
      if [[ "$NODE_SET" == true ]]; then
        echo "Error: node can only be specified once" >&2
        exit 1
      fi
      NODE="$arg"
      NODE_SET=true
      ;;
    *)
      usage
      ;;
  esac
done

case "$NODE" in
  dev|node2|node3) ;;
  *) echo "Error: node must be 'dev' or 'node2' or 'node3'" >&2; exit 1 ;;
esac

TF_VM_USERNAME="${TF_VM_USERNAME:-dummy}"
export TF_VM_USERNAME

TF_VM_SSH_PUBLIC_KEY="${TF_VM_SSH_PUBLIC_KEY:-dummy}"
export TF_VM_SSH_PUBLIC_KEY

OUT_DIR="${TF_DIR}/vm/${NODE}/${VM_NAME}"

if [[ ! -d "$OUT_DIR" ]]; then
  echo "Error: ${OUT_DIR} does not exist" >&2
  exit 1
fi

if [[ ! -f "${OUT_DIR}/terragrunt.hcl" ]]; then
  echo "Error: ${OUT_DIR}/terragrunt.hcl does not exist" >&2
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
