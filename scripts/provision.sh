#!/usr/bin/env bash
set -euo pipefail

# Provision an existing VM over SSH: install the CLI toolchain, terminal and
# fonts, wire up the shell init files, then fetch secrets (env and kubeconfig)
# from OpenBao.

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

SSH_OPTS=(-o StrictHostKeyChecking=accept-new -o ConnectTimeout=5 -o BatchMode=yes)
# All provisioning scripts are staged under this single directory so they can be
# removed in one shot when provisioning finishes.
REMOTE_ROOT="/tmp/homelab-provision"

# Stage every script a remote step needs under REMOTE_ROOT in a single
# round-trip, preserving each script's path relative to SCRIPT_DIR so it resolves
# its siblings the same way it does locally (install/* finds install/vendor,
# secrets/* finds ../lib). This phase handles no credentials. secrets/ ships only
# the get-* readers; the privileged admin/set-* scripts are never copied to a
# provisioned VM.
stage_scripts() {
  echo "Staging provisioning scripts on ${IP}..."
  # shellcheck disable=SC2029  # REMOTE_ROOT is a client-side constant, expanded here by design
  ssh "${SSH_OPTS[@]}" -n "${USERNAME}@${IP}" "rm -rf ${REMOTE_ROOT} && mkdir -p ${REMOTE_ROOT}"
  # --no-xattrs keeps macOS bsdtar from embedding extended attributes (e.g.
  # com.apple.provenance), which GNU tar on the Linux VM would warn about on
  # extract. Both bsdtar and GNU tar accept the flag.
  # shellcheck disable=SC2029
  tar --no-xattrs -C "$SCRIPT_DIR" -cf - \
        install lib \
        secrets/get-env.sh secrets/get-kubeconfig.sh \
    | ssh "${SSH_OPTS[@]}" "${USERNAME}@${IP}" "tar -C ${REMOTE_ROOT} -xf -"
}

# Execute a staged script on the VM. `rel` is its path relative to SCRIPT_DIR.
# Extra args are forwarded as `KEY=VALUE` env assignments. This is a single ssh,
# so any piped stdin (e.g. a credential) reaches the script intact -- there is no
# sibling ssh to consume it first.
run_remote() {
  local rel="$1"; shift
  # shellcheck disable=SC2029  # remote path and env assignments expand client-side by design
  ssh "${SSH_OPTS[@]}" "${USERNAME}@${IP}" "$* bash ${REMOTE_ROOT}/${rel}"
}

shell_quote() {
  printf "%q" "$1"
}

run_openbao_remote() {
  local rel="$1"
  local openbao_addr_remote
  local bao_username_remote

  openbao_addr_remote="$(shell_quote "$OPENBAO_ADDR")"
  bao_username_remote="$(shell_quote "$BAO_USERNAME")"

  if [[ -n "${BAO_TOKEN:-}" ]]; then
    printf '%s\n' "$BAO_TOKEN" | \
      run_remote "$rel" \
        "OPENBAO_ADDR=${openbao_addr_remote}" \
        "BAO_USERNAME=${bao_username_remote}" \
        "BAO_TOKEN_STDIN=1"
  else
    printf '%s\n' "$OPENBAO_PASSWORD" | \
      run_remote "$rel" \
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


# Remove the staged scripts from the VM when provisioning finishes (including on
# error), so no auth helper or script copies linger under /tmp.
trap 'ssh "${SSH_OPTS[@]}" -n "${USERNAME}@${IP}" "rm -rf ${REMOTE_ROOT}" 2>/dev/null || true' EXIT

# Stage all scripts in one round-trip. The install/*.sh wrappers run the bundled
# install/vendor/run_onchange_*.sh instead of fetching from GitHub, so the VM
# never depends on the GitHub API rate limit at this point.
stage_scripts

echo "Running tool installation..."
run_remote "install/tools.sh"

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
run_remote "install/terminal.sh"

echo "Running font installation..."
run_remote "install/fonts.sh"

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

# Run get-env.sh on the VM to populate ~/.env from OpenBao secrets
echo "Fetching env secrets from OpenBao..."
run_openbao_remote "secrets/get-env.sh"

# Run get-kubeconfig.sh on the VM to populate ~/.kube from OpenBao secrets
echo "Retrieving kubeconfig from OpenBao..."
run_openbao_remote "secrets/get-kubeconfig.sh"

echo ""
echo "=== Provisioning complete ==="
echo "Connect: ssh ${USERNAME}@${IP}"
echo ""
echo "=== Next step: generate SSH key on the VM ==="
echo "  ssh ${USERNAME}@${IP}"
echo "  ssh-keygen -t ed25519 -C '${USERNAME}@${IP}'"
