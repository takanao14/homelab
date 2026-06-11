#!/usr/bin/env bash
set -euo pipefail

# Provision a VM: verify/install system packages, install the CLI toolchain,
# terminal and fonts, wire up the shell init files, then fetch secrets (env and
# kubeconfig) from OpenBao.
#
# Two modes:
#   remote (default)  provision the VM at <ip> over SSH (push from this host)
#   --local           provision THIS machine directly, no SSH. Run it on the
#                     target Linux box as the user being provisioned.

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
Usage: $(basename "$0") <ip> [username]
       $(basename "$0") --local [username]

  ip        IPv4 address of the VM (remote mode)
  username  target username (default: \$USER)

Modes:
  remote (default)  provision the VM at <ip> over SSH
  --local           provision this machine directly (no SSH); must be run on
                    the target Linux box as the user being provisioned

Examples:
  $(basename "$0") 192.168.20.50 myuser
  $(basename "$0") --local
EOF
  exit 1
}

# --- Argument parsing -------------------------------------------------------
LOCAL_MODE=false
POSITIONAL=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    --local) LOCAL_MODE=true; shift ;;
    -h|--help) usage ;;
    --) shift; POSITIONAL+=("$@"); break ;;
    -*) echo "Unknown option: $1" >&2; usage ;;
    *) POSITIONAL+=("$1"); shift ;;
  esac
done
set -- "${POSITIONAL[@]}"

validate_local_target() {
  if [[ "$(uname -s)" != "Linux" ]]; then
    echo "Error: --local is supported only on Linux." >&2
    exit 1
  fi

  if [[ ! -r /etc/os-release ]]; then
    echo "Error: /etc/os-release not found." >&2
    exit 1
  fi

  local os_id
  os_id="$(
    # Read only ID instead of sourcing the system-owned file into this shell.
    # shellcheck disable=SC1091
    . /etc/os-release
    printf '%s' "$ID"
  )"
  case "$os_id" in
    ubuntu|debian|rocky) ;;
    *)
      echo "Error: unsupported Linux distribution: ${os_id}" >&2
      exit 1
      ;;
  esac
}

if $LOCAL_MODE; then
  if (( $# > 1 )); then
    echo "Error: too many arguments for --local." >&2
    usage
  fi

  validate_local_target

  # No SSH target in local mode; provisioning happens in place.
  IP="localhost"
  USERNAME="${1:-$USER}"
  # We never `su` to another user, so the requested user must be the caller.
  if [[ "$USERNAME" != "$USER" ]]; then
    echo "Error: --local provisions the current user only (got '${USERNAME}', running as '${USER}')." >&2
    echo "Re-run as that user, or omit the username." >&2
    exit 1
  fi
else
  if (( $# < 1 || $# > 2 )); then
    echo "Error: remote mode requires <ip> and optional [username]." >&2
    usage
  fi

  IP="$1"
  USERNAME="${2:-$USER}"
fi

SSH_OPTS=(-o StrictHostKeyChecking=accept-new -o ConnectTimeout=5 -o BatchMode=yes)
# All provisioning scripts are staged under this single directory so they can be
# removed in one shot when provisioning finishes (remote mode only).
REMOTE_ROOT="/tmp/homelab-provision"

# Stage every script a remote step needs under REMOTE_ROOT in a single
# round-trip, preserving each script's path relative to SCRIPT_DIR so it resolves
# its siblings the same way it does locally (install/* finds install/vendor,
# secrets/* finds ../lib). This phase handles no credentials. secrets/ ships only
# the get-* readers; the privileged admin/set-* scripts are never copied to a
# provisioned VM. Not used in --local mode: the scripts already live in SCRIPT_DIR.
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

# Execute a staged script. `rel` is its path relative to SCRIPT_DIR. Extra args
# are forwarded as `KEY=VALUE` env assignments. In remote mode this is a single
# ssh, so any piped stdin (e.g. a credential) reaches the script intact -- there
# is no sibling ssh to consume it first. In local mode the real script under
# SCRIPT_DIR is run directly, resolving its siblings the same way.
run_remote() {
  local rel="$1"; shift
  if $LOCAL_MODE; then
    env "$@" bash "${SCRIPT_DIR}/${rel}"
  else
    # shellcheck disable=SC2029  # remote path and env assignments expand client-side by design
    ssh "${SSH_OPTS[@]}" "${USERNAME}@${IP}" "$* bash ${REMOTE_ROOT}/${rel}"
  fi
}

# Run an arbitrary shell command on the target. The command string is evaluated
# by bash either locally or on the remote host, so `~`/`$HOME`/`$USER` expand for
# the target user in both modes.
run_shell() {
  if $LOCAL_MODE; then
    bash -c "$1"
  else
    # shellcheck disable=SC2029  # command string is evaluated by the target's bash by design
    ssh "${SSH_OPTS[@]}" "${USERNAME}@${IP}" "$1"
  fi
}

# Run a shell script supplied on stdin on the target (used for the kitty config
# heredoc, whose body must not be expanded by the client shell).
run_shell_stdin() {
  if $LOCAL_MODE; then
    bash -s
  else
    ssh "${SSH_OPTS[@]}" "${USERNAME}@${IP}" 'bash -s'
  fi
}

shell_quote() {
  printf "%q" "$1"
}

run_openbao_target() {
  local rel="$1"
  local openbao_addr_arg="$OPENBAO_ADDR"
  local bao_username_arg="$BAO_USERNAME"

  if ! $LOCAL_MODE; then
    openbao_addr_arg="$(shell_quote "$OPENBAO_ADDR")"
    bao_username_arg="$(shell_quote "$BAO_USERNAME")"
  fi

  if [[ -n "${BAO_TOKEN:-}" ]]; then
    printf '%s\n' "$BAO_TOKEN" | \
      run_remote "$rel" \
        "OPENBAO_ADDR=${openbao_addr_arg}" \
        "BAO_USERNAME=${bao_username_arg}" \
        "BAO_TOKEN_STDIN=1"
  else
    printf '%s\n' "$OPENBAO_PASSWORD" | \
      run_remote "$rel" \
        "OPENBAO_ADDR=${openbao_addr_arg}" \
        "BAO_USERNAME=${bao_username_arg}"
  fi
}

if $LOCAL_MODE; then
  echo "Provisioning this machine locally as ${USERNAME}..."
else
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

  # Remove the staged scripts from the VM when provisioning finishes (including
  # on error), so no auth helper or script copies linger under /tmp.
  trap 'ssh "${SSH_OPTS[@]}" -n "${USERNAME}@${IP}" "rm -rf ${REMOTE_ROOT}" 2>/dev/null || true' EXIT

  # Stage all scripts in one round-trip. The install/*.sh wrappers run the
  # bundled install/vendor/run_onchange_*.sh instead of fetching from GitHub, so
  # the VM never depends on the GitHub API rate limit at this point.
  stage_scripts
fi

# Wait for cloud-init to finish (no-op where cloud-init is not installed)
echo "Waiting for cloud-init to complete..."
run_shell "cloud-init status --wait" 2>/dev/null || true
echo "cloud-init complete."

if $LOCAL_MODE; then
  echo "Verifying system package prerequisites (no sudo)..."
  run_remote "install/packages.sh" "TOOL_SKIP_SYSTEM_PACKAGES=1"
else
  echo "Running system package installation..."
  run_remote "install/packages.sh"
fi

echo "Running tool installation..."
run_remote "install/tools.sh"

echo "Ensuring \$HOME/.local/bin is in PATH..."
run_shell \
  "grep -qF '\$HOME/.local/bin' ~/.bashrc || echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.bashrc"

echo "Ensuring ~/.env is sourced in ~/.bashrc..."
run_shell \
  "grep -qF '.env' ~/.bashrc || echo '[[ -f \"\$HOME/.env\" ]] && set -a && source \"\$HOME/.env\" && set +a' >> ~/.bashrc"

echo "Ensuring direnv hook is enabled in ~/.bashrc..."
run_shell \
  "grep -qF 'direnv hook bash' ~/.bashrc || echo 'command -v direnv >/dev/null 2>&1 && eval \"\$(direnv hook bash)\"' >> ~/.bashrc"

echo "Ensuring ~/.bash_profile sources ~/.bashrc..."
run_shell \
  "grep -qF '.bashrc' ~/.bash_profile 2>/dev/null || echo '[[ -f \"\$HOME/.bashrc\" ]] && source \"\$HOME/.bashrc\"' >> ~/.bash_profile"

echo "Running terminal installation..."
run_remote "install/terminal.sh"

echo "Running font installation..."
run_remote "install/fonts.sh"

echo "Configuring kitty font..."
run_shell_stdin <<'REMOTE'
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

# Run get-env.sh to populate ~/.env from OpenBao secrets
echo "Fetching env secrets from OpenBao..."
run_openbao_target "secrets/get-env.sh"

# Run get-kubeconfig.sh to populate ~/.kube from OpenBao secrets
echo "Retrieving kubeconfig from OpenBao..."
run_openbao_target "secrets/get-kubeconfig.sh"

echo ""
echo "=== Provisioning complete ==="
if $LOCAL_MODE; then
  echo "Open a new shell (or run: source ~/.bashrc) to pick up PATH and ~/.env."
else
  echo "Connect: ssh ${USERNAME}@${IP}"
  echo ""
  echo "=== Next step: generate SSH key on the VM ==="
  echo "  ssh ${USERNAME}@${IP}"
  echo "  ssh-keygen -t ed25519 -C '${USERNAME}@${IP}'"
fi
