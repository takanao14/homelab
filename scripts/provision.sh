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

SSH_OPTS="-o StrictHostKeyChecking=no -o ConnectTimeout=5 -o BatchMode=yes"

# Wait for SSH to become available
echo "Waiting for SSH on ${IP}..."
until ssh $SSH_OPTS "${USERNAME}@${IP}" "true" 2>/dev/null; do
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
scp -o StrictHostKeyChecking=no "$INSTALL_SCRIPT" "${USERNAME}@${IP}:/tmp/install-tools.sh"

echo "Running tool installation..."
ssh $SSH_OPTS "${USERNAME}@${IP}" "bash /tmp/install-tools.sh"

# TODO: OpenBao credential setup
# - bao login via AppRole (role_id + secret_id)
# - retrieve kubeconfig -> ~/.kube/config
# - retrieve API keys   -> ~/.config/...

echo ""
echo "=== Provisioning complete ==="
echo "Connect: ssh ${USERNAME}@${IP}"
