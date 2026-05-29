#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

usage() {
  cat <<EOF
Usage: $(basename "$0") <ip> <username>

  ip        IPv4 address of the VM
  username  SSH username (default: \$USER)

Example:
  $(basename "$0") 192.168.20.50 myuser
EOF
  exit 1
}

[[ $# -lt 1 ]] && usage

IP="$1"
USERNAME="${2:-$USER}"
INSTALL_SCRIPT="${SCRIPT_DIR}/vm-setup/install-tools.sh"

SSH_OPTS="-o StrictHostKeyChecking=accept-new -o ConnectTimeout=5 -o BatchMode=yes"

# Wait for SSH to become available (max 5 min)
echo "Waiting for SSH on ${IP}..."
max_attempts=60
attempts=0
until ssh $SSH_OPTS "${USERNAME}@${IP}" "true" 2>/dev/null; do
  (( attempts++ ))
  if (( attempts >= max_attempts )); then
    echo ""
    echo "Error: timed out waiting for SSH on ${IP}" >&2
    exit 1
  fi
  printf "."
  sleep 5
done
echo ""
echo "SSH is ready."

# Generate SSH key pair on VM if not present
ssh $SSH_OPTS "${USERNAME}@${IP}" \
  "[[ -f ~/.ssh/id_ed25519 ]] || ssh-keygen -t ed25519 -N '' -f ~/.ssh/id_ed25519 -q -C '${USERNAME}@${IP}'"

# Copy and run tool installation
echo "Copying install-tools.sh..."
scp -o StrictHostKeyChecking=accept-new "$INSTALL_SCRIPT" "${USERNAME}@${IP}:/tmp/install-tools.sh"

echo "Running tool installation..."
ssh $SSH_OPTS "${USERNAME}@${IP}" "bash /tmp/install-tools.sh"

OPENBAO_ADDR="${OPENBAO_ADDR:-}"
OPENBAO_USERNAME="${OPENBAO_USERNAME:-}"
OPENBAO_PASSWORD="${OPENBAO_PASSWORD:-}"

if [[ -n "$OPENBAO_ADDR" && -n "$OPENBAO_USERNAME" && -n "$OPENBAO_PASSWORD" ]]; then
  echo "Retrieving kubeconfig from OpenBao..."
  ssh $SSH_OPTS "${USERNAME}@${IP}" bash -s <<EOF
set -euo pipefail
export BAO_ADDR="${OPENBAO_ADDR}"
BAO_TOKEN=\$(bao login -method=userpass username="${OPENBAO_USERNAME}" password="${OPENBAO_PASSWORD}" -token-only)
export BAO_TOKEN
mkdir -p ~/.kube
bao kv get -field=kubeconfig secret/kubeconfig/dev > ~/.kube/dev-homelab.yaml
bao kv get -field=kubeconfig secret/kubeconfig/prd > ~/.kube/prd-homelab.yaml
chmod 600 ~/.kube/dev-homelab.yaml ~/.kube/prd-homelab.yaml
EOF
  echo "Kubeconfig retrieved."
fi

echo ""
echo "=== Provisioning complete ==="
echo "Connect: ssh ${USERNAME}@${IP}"
echo ""
echo "=== VM public key (register where needed e.g. GitHub) ==="
ssh $SSH_OPTS "${USERNAME}@${IP}" "cat ~/.ssh/id_ed25519.pub"
