#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

ENV_FILE="${HOME}/.env"
if [[ -f "$ENV_FILE" ]]; then
  set -a; source "$ENV_FILE"; set +a
fi

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
INSTALL_SCRIPT="${SCRIPT_DIR}/install-tools.sh"
TERMINAL_SCRIPT="${SCRIPT_DIR}/install-terminal.sh"
FONTS_SCRIPT="${SCRIPT_DIR}/install-fonts.sh"
KUBECONFIG_SCRIPT="${SCRIPT_DIR}/get-kubeconfig.sh"

SSH_OPTS="-o StrictHostKeyChecking=accept-new -o ConnectTimeout=5 -o BatchMode=yes"

# Copy a script to the VM and execute it remotely.
# Any extra args are forwarded as `KEY=VALUE` env assignments. Remaining stdin
# (e.g. a piped secret) is passed through to the remote process.
run_remote() {
  local script="$1"; shift
  local base; base="$(basename "$script")"
  scp $SSH_OPTS "$script" "${USERNAME}@${IP}:/tmp/${base}"
  ssh $SSH_OPTS "${USERNAME}@${IP}" "$* bash /tmp/${base}"
}

# Wait for SSH to become available (max 5 min)
echo "Waiting for SSH on ${IP}..."
max_attempts=60
attempts=0
until ssh $SSH_OPTS "${USERNAME}@${IP}" "true" 2>/dev/null; do
  attempts=$(( attempts + 1 ))
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
echo "Running tool installation..."
run_remote "$INSTALL_SCRIPT"

echo "Ensuring \$HOME/.local/bin is in PATH..."
ssh $SSH_OPTS "${USERNAME}@${IP}" \
  "grep -qF '\$HOME/.local/bin' ~/.bashrc || echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.bashrc"

echo "Running terminal installation..."
run_remote "$TERMINAL_SCRIPT"

echo "Running font installation..."
run_remote "$FONTS_SCRIPT"

echo "Configuring kitty font..."
ssh $SSH_OPTS "${USERNAME}@${IP}" "
  mkdir -p ~/.config/kitty
  cat >> ~/.config/kitty/kitty.conf <<'EOF'

# font
font_family      UDEV Gothic NFLG
bold_font        UDEV Gothic NFLG Bold
italic_font      UDEV Gothic NFLG Italic
bold_italic_font UDEV Gothic NFLG Bold Italic
font_size 12.0
EOF
"

OPENBAO_ADDR="${OPENBAO_ADDR:-https://openbao.home.butaco.net}"
BAO_USERNAME="${BAO_USERNAME:-homelab}"

read -rsp "OpenBao password for ${BAO_USERNAME}: " OPENBAO_PASSWORD; echo

# Run get-kubeconfig.sh on the VM, feeding the password via stdin
echo "Retrieving kubeconfig from OpenBao..."
printf '%s\n' "$OPENBAO_PASSWORD" | \
  run_remote "$KUBECONFIG_SCRIPT" "OPENBAO_ADDR='${OPENBAO_ADDR}' BAO_USERNAME='${BAO_USERNAME}'"

echo ""
echo "=== Provisioning complete ==="
echo "Connect: ssh ${USERNAME}@${IP}"
echo ""
echo "=== VM public key (register where needed e.g. GitHub) ==="
ssh $SSH_OPTS "${USERNAME}@${IP}" "cat ~/.ssh/id_ed25519.pub"
