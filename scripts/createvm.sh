#!/usr/bin/env bash
set -euo pipefail

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
Usage: $(basename "$0") <name> <ip> [node] [cores] [memory_mb] [disk_gb] [image]

  name      VM name (alphanumeric and hyphens only)
  ip        IPv4 address without prefix (e.g. 192.168.20.50)
  node      Proxmox node: dev | node2 | node3 (default: dev)
  cores     vCPUs                      (default: 4)
  memory    Memory in MB               (default: 8192)
  disk      Disk size in GB            (default: 80)
  image     OS image: ubuntu24 | ubuntu24-xrdp | rocky10 | rocky9-xrdp  (default: ubuntu24)

Required env vars: TF_VM_USERNAME, TF_VM_PASSWORD, TF_VM_SSH_PUBLIC_KEY

Example:
  $(basename "$0") myvm 192.168.20.50
  $(basename "$0") myvm 192.168.20.50 dev 4 4096 80 rocky10
EOF
  exit 1
}

[[ $# -lt 2 || $# -gt 7 ]] && usage

VM_NAME="$1"

if [[ ! "$VM_NAME" =~ ^[a-zA-Z0-9-]+$ ]]; then
  echo "Error: VM name must contain only alphanumeric characters and hyphens" >&2
  exit 1
fi

IP="$2"
NODE="${3:-dev}"
CORES="${4:-4}"
MEMORY="${5:-8192}"
DISK="${6:-80}"
IMAGE="${7:-ubuntu24}"

case "$NODE" in
  dev|node2|node3) ;;
  *) echo "Error: node must be 'dev' or 'node2' or 'node3'" >&2; exit 1 ;;
esac

if [[ ! "$IP" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]]; then
  echo "Error: IP must be an IPv4 address without prefix" >&2
  exit 1
fi

IFS=. read -r octet1 octet2 octet3 octet4 <<<"$IP"
for octet in "$octet1" "$octet2" "$octet3" "$octet4"; do
  if (( 10#$octet > 255 )); then
    echo "Error: IP octet out of range: ${IP}" >&2
    exit 1
  fi
done

for value_name in CORES MEMORY DISK; do
  value="${!value_name}"
  if [[ ! "$value" =~ ^[1-9][0-9]*$ ]]; then
    echo "Error: ${value_name,,} must be a positive integer" >&2
    exit 1
  fi
done

case "$IMAGE" in
  ubuntu24) FILE_ID="local:iso/ubuntu-24.04-custom.img" ;;
  ubuntu24-xrdp) FILE_ID="local:iso/ubuntu-24.04-xrdp.img" ;;
  rocky10)  FILE_ID="local:iso/rocky-10-custom.img" ;;
  rocky9-xrdp)  FILE_ID="local:iso/rocky-9-xrdp.img" ;;
  *) echo "Error: image must be 'ubuntu24', 'ubuntu24-xrdp', 'rocky10', or 'rocky9-xrdp'" >&2; exit 1 ;;
esac

SUBNET=$(echo "$IP" | cut -d. -f1-3)
case "$SUBNET" in
  192.168.10) NET_REF="local.common.locals.${NODE}.net10" ;;
  192.168.20) NET_REF="local.common.locals.dev.net20" ;;
  192.168.40) NET_REF="local.common.locals.node2.net40" ;;
  192.168.50) NET_REF="local.common.locals.node3.net50" ;;
  *) echo "Error: unrecognized subnet ${SUBNET}.0/24" >&2; exit 1 ;;
esac

case "${NODE}:${SUBNET}" in
  dev:192.168.10|dev:192.168.20|node2:192.168.10|node2:192.168.40|node3:192.168.10|node3:192.168.50) ;;
  *) echo "Error: subnet ${SUBNET}.0/24 is not available on node '${NODE}'" >&2; exit 1 ;;
esac

NODE_UPPER="${NODE^^}"
_node_var() { local var="${1}_${NODE_UPPER}"; echo "${!var:-}"; }

TF_VM_USERNAME_DEFAULT="${TF_VM_USERNAME:-}"
TF_VM_PASSWORD_DEFAULT="${TF_VM_PASSWORD:-}"
TF_VM_SSH_PUBLIC_KEY_DEFAULT="${TF_VM_SSH_PUBLIC_KEY:-}"

TF_VM_USERNAME="$(_node_var TF_VM_USERNAME)"
TF_VM_USERNAME="${TF_VM_USERNAME:-$TF_VM_USERNAME_DEFAULT}"
TF_VM_PASSWORD="$(_node_var TF_VM_PASSWORD)"
TF_VM_PASSWORD="${TF_VM_PASSWORD:-$TF_VM_PASSWORD_DEFAULT}"
TF_VM_SSH_PUBLIC_KEY_NODE="$(_node_var TF_VM_SSH_PUBLIC_KEY)"
TF_VM_SSH_PUBLIC_KEY="${TF_VM_SSH_PUBLIC_KEY_NODE:-$TF_VM_SSH_PUBLIC_KEY_DEFAULT}"

if [[ -z "$TF_VM_USERNAME" ]]; then
  TF_VM_USERNAME="$USER"
fi
export TF_VM_USERNAME

if [[ -z "${TF_VM_SSH_PUBLIC_KEY:-}" ]]; then
  DEFAULT_PUBKEY="${HOME}/.ssh/id_ed25519.pub"
  [[ ! -f "$DEFAULT_PUBKEY" ]] && echo "Error: \$TF_VM_SSH_PUBLIC_KEY is not set and ${DEFAULT_PUBKEY} not found" >&2 && exit 1
  TF_VM_SSH_PUBLIC_KEY="$(cat "$DEFAULT_PUBKEY")"
  export TF_VM_SSH_PUBLIC_KEY
fi

OUT_DIR="${TF_DIR}/vm/${NODE}/${VM_NAME}"

if [[ -d "$OUT_DIR" ]]; then
  echo "Error: ${OUT_DIR} already exists" >&2
  exit 1
fi

if [[ -z "$TF_VM_PASSWORD" ]]; then
  read -rsp "VM password (${NODE}): " TF_VM_PASSWORD
  echo ""
fi
export TF_VM_PASSWORD

mkdir -p "$OUT_DIR"

cat > "${OUT_DIR}/terragrunt.hcl" <<HCL
include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "\${get_parent_terragrunt_dir()}/modules/proxmox-vm"
}

locals {
  env    = read_terragrunt_config(find_in_parent_folders("env.hcl"))
  common = read_terragrunt_config(find_in_parent_folders("common.hcl"))

  base_vars = merge(local.env.locals.vm_defaults, {
    dns_servers = local.common.locals.dns_internal
    dns_domain  = local.common.locals.dns_domain
  })
}

inputs = {
  vms = {
    "${VM_NAME}" = merge(local.base_vars, {
      cores  = ${CORES}
      memory = ${MEMORY}
      bridge = ${NET_REF}.bridge
      ipv4   = "${IP}/24"
      ipv4gw = ${NET_REF}.ipv4gw
      disks = {
        scsi0 = merge(local.env.locals.disk_defaults, {
          size    = ${DISK}
          file_id = "${FILE_ID}"
        })
      }
    })
  }
}
HCL

echo ""
echo "Generated: tf/vm/${NODE}/${VM_NAME}/terragrunt.hcl"
echo "---"
cat "${OUT_DIR}/terragrunt.hcl"
echo "---"
echo ""

read -r -p "Apply? (y/N) " confirm
if [[ "${confirm,,}" != "y" ]]; then
  echo "Aborted. Generated file remains at tf/vm/${NODE}/${VM_NAME}/terragrunt.hcl"
  exit 0
fi

direnv exec "$OUT_DIR" bash -c "cd '$OUT_DIR' && terragrunt apply"

echo ""
echo "Removing ${IP} from known_hosts..."
ssh-keygen -R "${IP}" 2>/dev/null || true

echo -n "Waiting for SSH on ${IP} ..."
TIMEOUT=300
ELAPSED=0
while true; do
  set +e
  ssh -o StrictHostKeyChecking=no -o ConnectTimeout=5 -o BatchMode=yes \
      -i "${HOME}/.ssh/id_ed25519" "${TF_VM_USERNAME}@${IP}" true 2>/dev/null
  SSH_EXIT=$?
  set -e
  if [[ $SSH_EXIT -eq 0 ]]; then
    break
  fi
  if [[ $ELAPSED -ge $TIMEOUT ]]; then
    echo ""
    echo "Error: SSH on ${IP} did not become ready within ${TIMEOUT}s" >&2
    exit 1
  fi
  printf '.'
  sleep 5
  ELAPSED=$((ELAPSED + 5))
done
echo " ready (${ELAPSED}s)"
