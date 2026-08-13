#!/usr/bin/env bash
set -euo pipefail

# Generate a Terragrunt config under tf/vm/<node>/<name>/.
# Planning, applying, and provisioning are intentionally separate operations.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TF_DIR="${SCRIPT_DIR}/../tf"

usage() {
  cat <<EOF
Usage: $(basename "$0") <name> <ip> [node] [cores] [memory_mb] [disk_gb] [image]

  name      VM name (alphanumeric and hyphens only)
  ip        IPv4 address without prefix (e.g. 192.168.20.50)
  node      Proxmox node: pve | node2 | node3 | node4 (default: pve)
  cores     vCPUs                      (default: 4)
  memory    Memory in MB               (default: 8192)
  disk      Disk size in GB            (default: 80)
  image     OS image: ubuntu24 | ubuntu24-xrdp | rocky10 | rocky9 | rocky9-xrdp | debian13  (default: ubuntu24)

Environment:
  TF_VM_USERNAME        VM username     (default: current username)
  TF_VM_PASSWORD        VM password     (prompted when unset)
  TF_VM_SSH_PUBLIC_KEY  SSH public key  (default: ~/.ssh/id_ed25519.pub)

Example:
  $(basename "$0") myvm 192.168.20.50
  $(basename "$0") myvm 192.168.20.50 pve 4 4096 80 rocky10
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
NODE="${3:-pve}"
CORES="${4:-4}"
MEMORY="${5:-8192}"
DISK="${6:-80}"
IMAGE="${7:-ubuntu24}"

case "$NODE" in
  pve|node2|node3|node4) ;;
  *) echo "Error: node must be one of: pve, node2, node3, node4" >&2; exit 1 ;;
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

# CI verifies these filenames against the Terraform image definitions.
case "$IMAGE" in
  ubuntu24) FILE_ID="local:iso/ubuntu-24.04-custom.img" ;;
  ubuntu24-xrdp) FILE_ID="local:iso/ubuntu-24.04-xrdp.img" ;;
  rocky10)  FILE_ID="local:iso/rocky-10-custom.img" ;;
  rocky9-xrdp)  FILE_ID="local:iso/rocky-9-xrdp.img" ;;
  rocky9)  FILE_ID="local:iso/rocky-9-custom.img" ;;
  debian13)  FILE_ID="local:iso/debian-13-custom.img" ;;
  *) echo "Error: image must be one of: ubuntu24, ubuntu24-xrdp, rocky10, rocky9, rocky9-xrdp, debian13" >&2; exit 1 ;;
esac

case "$IMAGE" in
  ubuntu24-xrdp|rocky9-xrdp)
    if [[ "$NODE" != "pve" ]]; then
      echo "Error: image '${IMAGE}' is only available on node 'pve'" >&2
      exit 1
    fi
    ;;
esac

SUBNET=$(echo "$IP" | cut -d. -f1-3)
case "$SUBNET" in
  192.168.10) NET_REF="local.common.locals.${NODE}.net10" ;;
  192.168.20) NET_REF="local.common.locals.pve.net20" ;;
  192.168.40) NET_REF="local.common.locals.node2.net40" ;;
  192.168.50) NET_REF="local.common.locals.node3.net50" ;;
  192.168.60) NET_REF="local.common.locals.node4.net60" ;;
  *) echo "Error: unrecognized subnet ${SUBNET}.0/24" >&2; exit 1 ;;
esac

case "${NODE}:${SUBNET}" in
  pve:192.168.10|pve:192.168.20|node2:192.168.10|node2:192.168.40|node3:192.168.10|node3:192.168.50|node4:192.168.10|node4:192.168.60) ;;
  *) echo "Error: subnet ${SUBNET}.0/24 is not available on node '${NODE}'" >&2; exit 1 ;;
esac

OUT_DIR="${TF_DIR}/vm/${NODE}/${VM_NAME}"
OUT_FILE="${OUT_DIR}/terragrunt.hcl"
OUT_ENVRC="${OUT_DIR}/.envrc"
OUT_GITIGNORE="${OUT_DIR}/.gitignore"

# Reuse saved credentials; explicit environment values take precedence.
if [[ -f "$OUT_ENVRC" ]]; then
  explicit_username="${TF_VM_USERNAME:-}"
  explicit_password="${TF_VM_PASSWORD:-}"
  explicit_ssh_public_key="${TF_VM_SSH_PUBLIC_KEY:-}"

  # Ignore the generated direnv parent lookup while loading credentials.
  # shellcheck disable=SC2329
  source_up() { :; }
  # shellcheck disable=SC1090
  source "$OUT_ENVRC"
  unset -f source_up

  [[ -z "$explicit_username" ]] || TF_VM_USERNAME="$explicit_username"
  [[ -z "$explicit_password" ]] || TF_VM_PASSWORD="$explicit_password"
  [[ -z "$explicit_ssh_public_key" ]] || TF_VM_SSH_PUBLIC_KEY="$explicit_ssh_public_key"
fi

TF_VM_USERNAME="${TF_VM_USERNAME:-$(id -un)}"
TF_VM_SSH_PUBLIC_KEY="${TF_VM_SSH_PUBLIC_KEY:-${HOME}/.ssh/id_ed25519.pub}"

if [[ -z "${TF_VM_PASSWORD:-}" ]]; then
  if [[ ! -t 0 ]]; then
    echo "Error: TF_VM_PASSWORD is unset and no terminal is available for input" >&2
    exit 1
  fi

  read -rsp "VM password: " TF_VM_PASSWORD
  echo
  if [[ -z "$TF_VM_PASSWORD" ]]; then
    echo "Error: VM password must not be empty" >&2
    exit 1
  fi
fi

if [[ ! -f "$TF_VM_SSH_PUBLIC_KEY" ]]; then
  echo "Error: SSH public key not found: ${TF_VM_SSH_PUBLIC_KEY}" >&2
  exit 1
fi

export TF_VM_USERNAME TF_VM_PASSWORD TF_VM_SSH_PUBLIC_KEY

TMP_FILE="$(mktemp "${TMPDIR:-/tmp}/create-vm.XXXXXX")"
TMP_ENVRC=""
cleanup() {
  rm -f "$TMP_FILE"
  [[ -z "$TMP_ENVRC" ]] || rm -f "$TMP_ENVRC"
}
trap cleanup EXIT

cat > "$TMP_FILE" <<HCL
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

if [[ -f "$OUT_FILE" ]]; then
  if cmp -s "$OUT_FILE" "$TMP_FILE"; then
    echo "Already up to date: tf/vm/${NODE}/${VM_NAME}/terragrunt.hcl"
  else
    echo "Error: ${OUT_FILE} already exists with different content" >&2
    diff -u "$OUT_FILE" "$TMP_FILE" || true
    exit 1
  fi
else
  mkdir -p "$OUT_DIR"
  mv "$TMP_FILE" "$OUT_FILE"
  echo "Generated: tf/vm/${NODE}/${VM_NAME}/terragrunt.hcl"
fi

# Ignore .envrc before writing plaintext credentials.
if [[ ! -f "$OUT_GITIGNORE" ]]; then
  printf '.envrc\n' > "$OUT_GITIGNORE"
  echo "Generated: tf/vm/${NODE}/${VM_NAME}/.gitignore"
elif ! grep -Fxq '.envrc' "$OUT_GITIGNORE"; then
  printf '\n.envrc\n' >> "$OUT_GITIGNORE"
  echo "Updated: tf/vm/${NODE}/${VM_NAME}/.gitignore"
fi

if [[ ! -f "$OUT_ENVRC" ]]; then
  TMP_ENVRC="$(mktemp "${TMPDIR:-/tmp}/create-vm-envrc.XXXXXX")"
  {
    printf 'source_up\n\n'
    printf 'export TF_VM_USERNAME=%q\n' "$TF_VM_USERNAME"
    printf 'export TF_VM_PASSWORD=%q\n' "$TF_VM_PASSWORD"
    printf 'export TF_VM_SSH_PUBLIC_KEY=%q\n' "$TF_VM_SSH_PUBLIC_KEY"
  } > "$TMP_ENVRC"
  chmod 600 "$TMP_ENVRC"
  mv "$TMP_ENVRC" "$OUT_ENVRC"
  TMP_ENVRC=""
  echo "Generated local credentials: tf/vm/${NODE}/${VM_NAME}/.envrc"
else
  echo "Using existing local credentials: tf/vm/${NODE}/${VM_NAME}/.envrc"
fi

echo ""
echo "---"
cat "$OUT_FILE"
echo "---"
echo ""
echo "Next steps:"
echo "  cd ${OUT_DIR}"
echo "  direnv exec . terragrunt plan"
echo "  direnv exec . terragrunt apply"
echo "  ${SCRIPT_DIR}/provision.sh ${IP}"
