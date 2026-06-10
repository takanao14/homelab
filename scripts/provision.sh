#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

ENV_FILE="${HOME}/.env"
if [[ -f "$ENV_FILE" ]]; then
  set -a
  # shellcheck source=/dev/null
  source "$ENV_FILE"
  set +a
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
INSTALL_SCRIPT="${SCRIPT_DIR}/install/install-tools.sh"
TERMINAL_SCRIPT="${SCRIPT_DIR}/install/install-terminal.sh"
FONTS_SCRIPT="${SCRIPT_DIR}/install/install-fonts.sh"
KUBECONFIG_SCRIPT="${SCRIPT_DIR}/secrets/get-kubeconfig.sh"
GETENV_SCRIPT="${SCRIPT_DIR}/secrets/getenv.sh"
OPENBAO_AUTH_SCRIPT="${SCRIPT_DIR}/lib/openbao-auth.sh"
VENDOR_DIR="${SCRIPT_DIR}/install/vendor"

SSH_OPTS=(-o StrictHostKeyChecking=accept-new -o ConnectTimeout=5 -o BatchMode=yes)

# Copy a script to the VM and execute it remotely. The script's path relative to
# SCRIPT_DIR is mirrored under /tmp, so a script resolves its siblings the same
# way it does locally (e.g. install/* finds install/vendor, secrets/* finds
# ../lib). Any extra args are forwarded as `KEY=VALUE` env assignments. Remaining
# stdin (e.g. a piped secret) is passed through to the remote process.
run_remote() {
  local script="$1"; shift
  local rel="${script#"${SCRIPT_DIR}/"}"
  local remote="/tmp/${rel}"
  # shellcheck disable=SC2029  # dirname must expand client-side to build the path
  ssh "${SSH_OPTS[@]}" "${USERNAME}@${IP}" "mkdir -p $(dirname "$remote")"
  scp "${SSH_OPTS[@]}" "$script" "${USERNAME}@${IP}:${remote}"
  # shellcheck disable=SC2029
  ssh "${SSH_OPTS[@]}" "${USERNAME}@${IP}" "$* bash ${remote}"
}

shell_quote() {
  printf "%q" "$1"
}

run_openbao_remote() {
  local script="$1"
  local openbao_addr_remote
  local bao_username_remote

  openbao_addr_remote="$(shell_quote "$OPENBAO_ADDR")"
  bao_username_remote="$(shell_quote "$BAO_USERNAME")"

  if [[ -n "${BAO_TOKEN:-}" ]]; then
    printf '%s\n' "$BAO_TOKEN" | \
      run_remote "$script" \
        "OPENBAO_ADDR=${openbao_addr_remote}" \
        "BAO_USERNAME=${bao_username_remote}" \
        "BAO_TOKEN_STDIN=1"
  else
    printf '%s\n' "$OPENBAO_PASSWORD" | \
      run_remote "$script" \
        "OPENBAO_ADDR=${openbao_addr_remote}" \
        "BAO_USERNAME=${bao_username_remote}"
  fi
}

# Wait for SSH to become available (max 5 min)
echo "Waiting for SSH on ${IP}..."
max_attempts=60
attempts=0
until ssh "${SSH_OPTS[@]}" "${USERNAME}@${IP}" "true" 2>/dev/null; do
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

# Wait for cloud-init to finish
echo "Waiting for cloud-init to complete..."
ssh "${SSH_OPTS[@]}" "${USERNAME}@${IP}" "cloud-init status --wait" 2>/dev/null || true
echo "cloud-init complete."


# Copy the vendored installers next to where the wrappers land (/tmp/install).
# The install-*.sh wrappers run install/vendor/run_onchange_*.sh instead of
# fetching from GitHub, so the VM never depends on the GitHub API rate limit at
# this point.
echo "Copying vendored installers..."
ssh "${SSH_OPTS[@]}" "${USERNAME}@${IP}" "mkdir -p /tmp/install/vendor"
scp "${SSH_OPTS[@]}" "${VENDOR_DIR}"/run_onchange_*.sh "${USERNAME}@${IP}:/tmp/install/vendor/"

echo "Copying OpenBao auth helper..."
ssh "${SSH_OPTS[@]}" "${USERNAME}@${IP}" "mkdir -p /tmp/lib"
scp "${SSH_OPTS[@]}" "$OPENBAO_AUTH_SCRIPT" "${USERNAME}@${IP}:/tmp/lib/"

# Copy and run tool installation
echo "Running tool installation..."
run_remote "$INSTALL_SCRIPT"

echo "Ensuring \$HOME/.local/bin is in PATH..."
ssh "${SSH_OPTS[@]}" "${USERNAME}@${IP}" \
  "grep -qF '\$HOME/.local/bin' ~/.bashrc || echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.bashrc"

echo "Ensuring ~/.env is sourced in ~/.bashrc..."
ssh "${SSH_OPTS[@]}" "${USERNAME}@${IP}" \
  "grep -qF '.env' ~/.bashrc || echo '[[ -f \"\$HOME/.env\" ]] && set -a && source \"\$HOME/.env\" && set +a' >> ~/.bashrc"

echo "Ensuring direnv hook is enabled in ~/.bashrc..."
ssh "${SSH_OPTS[@]}" "${USERNAME}@${IP}" \
  "grep -qF 'direnv hook bash' ~/.bashrc || echo 'command -v direnv >/dev/null 2>&1 && eval \"\$(direnv hook bash)\"' >> ~/.bashrc"

echo "Ensuring ~/.bash_profile sources ~/.bashrc..."
ssh "${SSH_OPTS[@]}" "${USERNAME}@${IP}" \
  "grep -qF '.bashrc' ~/.bash_profile 2>/dev/null || echo '[[ -f \"\$HOME/.bashrc\" ]] && source \"\$HOME/.bashrc\"' >> ~/.bash_profile"

echo "Running terminal installation..."
run_remote "$TERMINAL_SCRIPT"

echo "Running font installation..."
run_remote "$FONTS_SCRIPT"

echo "Configuring kitty font..."
ssh "${SSH_OPTS[@]}" "${USERNAME}@${IP}" 'bash -s' <<'REMOTE'
set -euo pipefail

conf="${HOME}/.config/kitty/kitty.conf"
tmp="$(mktemp)"
cleanup() {
  rm -f "$tmp"
}
trap cleanup EXIT

mkdir -p "$(dirname "$conf")"
if [[ -f "$conf" ]]; then
  awk '
    $0 == "# BEGIN homelab font" { skip = 1; next }
    $0 == "# END homelab font" { skip = 0; next }
    !skip { print }
  ' "$conf" > "$tmp"
else
  : > "$tmp"
fi

cat >> "$tmp" <<'EOF'

# BEGIN homelab font
font_family      UDEV Gothic NFLG
bold_font        UDEV Gothic NFLG Bold
italic_font      UDEV Gothic NFLG Italic
bold_italic_font UDEV Gothic NFLG Bold Italic
font_size 12.0
# END homelab font
EOF

mv "$tmp" "$conf"
trap - EXIT
REMOTE

OPENBAO_ADDR="${OPENBAO_ADDR:-https://openbao.home.butaco.net}"
BAO_USERNAME="${BAO_USERNAME:-homelab}"

if [[ -z "${BAO_TOKEN:-}" ]]; then
  read -rsp "OpenBao password for ${BAO_USERNAME}: " OPENBAO_PASSWORD; echo
fi

# Run getenv.sh on the VM to populate ~/.env from OpenBao secrets
echo "Fetching env secrets from OpenBao..."
run_openbao_remote "$GETENV_SCRIPT"

# Run get-kubeconfig.sh on the VM using the same OpenBao credentials
echo "Retrieving kubeconfig from OpenBao..."
run_openbao_remote "$KUBECONFIG_SCRIPT"

echo ""
echo "=== Provisioning complete ==="
echo "Connect: ssh ${USERNAME}@${IP}"
echo ""
echo "=== Next step: generate SSH key on the VM ==="
echo "  ssh ${USERNAME}@${IP}"
echo "  ssh-keygen -t ed25519 -C '${USERNAME}@${IP}'"
