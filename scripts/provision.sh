#!/usr/bin/env bash
set -euo pipefail

# Provision a VM locally or over SSH, then fetch its OpenBao secrets.

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

Options:
  --profile desktop|server|auto
             machine profile override (default: auto; read from
             /etc/homelab/machine-profile, otherwise server)

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
REQUESTED_MACHINE_PROFILE="${TOOL_MACHINE_PROFILE:-auto}"
POSITIONAL=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    --local) LOCAL_MODE=true; shift ;;
    --profile)
      if (( $# < 2 )); then
        echo "Error: --profile requires desktop, server, or auto." >&2
        usage
      fi
      REQUESTED_MACHINE_PROFILE="$2"
      shift 2
      ;;
    -h|--help) usage ;;
    --) shift; POSITIONAL+=("$@"); break ;;
    -*) echo "Unknown option: $1" >&2; usage ;;
    *) POSITIONAL+=("$1"); shift ;;
  esac
done
set -- "${POSITIONAL[@]}"

case "$REQUESTED_MACHINE_PROFILE" in
  desktop|server|auto) ;;
  *)
    echo "Error: --profile must be desktop, server, or auto (got: ${REQUESTED_MACHINE_PROFILE})." >&2
    exit 1
    ;;
esac

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

  IP="localhost"
  USERNAME="${1:-$USER}"
  # Local mode never changes users.
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
# Remote scripts share one disposable staging directory.
REMOTE_ROOT="/tmp/homelab-provision"
CLOUD_INIT_WAIT_TIMEOUT="${CLOUD_INIT_WAIT_TIMEOUT:-600}"

# Stage required scripts once, preserving relative paths; exclude admin writers.
stage_scripts() {
  echo "Staging provisioning scripts on ${IP}..."
  # shellcheck disable=SC2029  # REMOTE_ROOT is a client-side constant, expanded here by design
  ssh "${SSH_OPTS[@]}" -n "${USERNAME}@${IP}" "rm -rf ${REMOTE_ROOT} && mkdir -p ${REMOTE_ROOT}"
  # Avoid macOS extended attributes in the Linux archive.
  # shellcheck disable=SC2029
  tar --no-xattrs -C "$SCRIPT_DIR" -cf - \
        install lib \
        secrets/get-env.sh secrets/get-kubeconfig.sh \
    | ssh "${SSH_OPTS[@]}" "${USERNAME}@${IP}" "tar -C ${REMOTE_ROOT} -xf -"
}

# Run a relative script with environment assignments, preserving piped stdin.
run_remote() {
  local rel="$1"; shift
  if $LOCAL_MODE; then
    env "$@" bash "${SCRIPT_DIR}/${rel}"
  else
    # shellcheck disable=SC2029  # remote path and env assignments expand client-side by design
    ssh "${SSH_OPTS[@]}" "${USERNAME}@${IP}" "$* bash ${REMOTE_ROOT}/${rel}"
  fi
}

# Run a command with target-side shell expansion.
run_shell() {
  if $LOCAL_MODE; then
    bash -c "$1"
  else
    # shellcheck disable=SC2029  # command string is evaluated by the target's bash by design
    ssh "${SSH_OPTS[@]}" "${USERNAME}@${IP}" "$1"
  fi
}

# Run a stdin script without client-side expansion.
run_shell_stdin() {
  if $LOCAL_MODE; then
    env "$@" bash -s
  else
    local env_prefix=""
    if (( $# > 0 )); then
      env_prefix="$* "
    fi
    # shellcheck disable=SC2029  # env_prefix is built client-side by design.
    ssh "${SSH_OPTS[@]}" "${USERNAME}@${IP}" "${env_prefix}bash -s"
  fi
}

wait_cloud_init() {
  echo "Waiting for cloud-init to complete..."
  run_shell_stdin "CLOUD_INIT_WAIT_TIMEOUT=$(shell_quote "$CLOUD_INIT_WAIT_TIMEOUT")" <<'REMOTE'
set -euo pipefail

timeout_seconds="${CLOUD_INIT_WAIT_TIMEOUT:-600}"
deadline=$(( $(date +%s) + timeout_seconds ))
boot_finished="/var/lib/cloud/instance/boot-finished"
last_status=""

if ! command -v cloud-init >/dev/null 2>&1; then
  echo "cloud-init is not installed; skipping wait."
  exit 0
fi

while true; do
  if [[ -f "$boot_finished" ]]; then
    cloud-init status --long 2>/dev/null || true
    echo "cloud-init boot-finished marker found."
    exit 0
  fi

  status="$(cloud-init status 2>&1 || true)"
  case "$status" in
    *"status: done"*)
      echo "$status"
      exit 0
      ;;
    *"status: disabled"*)
      echo "$status"
      exit 0
      ;;
    *"status: error"*)
      echo "$status" >&2
      exit 1
      ;;
  esac

  if [[ "$status" != "$last_status" ]]; then
    echo "$status"
    last_status="$status"
  fi

  if (( $(date +%s) >= deadline )); then
    echo "Error: timed out waiting for cloud-init after ${timeout_seconds}s." >&2
    echo "Last cloud-init status:" >&2
    echo "$status" >&2
    if command -v systemctl >/dev/null 2>&1; then
      systemctl --no-pager --full status cloud-init-local.service cloud-init.service cloud-config.service cloud-final.service >&2 || true
    fi
    exit 1
  fi

  sleep 5
done
REMOTE
  echo "cloud-init complete."
}

shell_quote() {
  printf "%q" "$1"
}

resolve_machine_profile() {
  if [[ "$REQUESTED_MACHINE_PROFILE" != "auto" ]]; then
    MACHINE_PROFILE="$REQUESTED_MACHINE_PROFILE"
    return
  fi

  MACHINE_PROFILE="$(run_shell_stdin <<'REMOTE'
set -euo pipefail

profile_file="/etc/homelab/machine-profile"
if [[ -r "$profile_file" ]]; then
  cat "$profile_file"
  exit 0
fi

echo server
REMOTE
)"

  case "$MACHINE_PROFILE" in
    desktop|server) ;;
    *)
      echo "Error: invalid machine profile from target: ${MACHINE_PROFILE}" >&2
      exit 1
      ;;
  esac
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
  # Wait up to five minutes for SSH.
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

  # Remove staged scripts on success or failure.
  trap 'ssh "${SSH_OPTS[@]}" -n "${USERNAME}@${IP}" "rm -rf ${REMOTE_ROOT}" 2>/dev/null || true' EXIT

  # Stage once; installers use bundled vendor scripts.
  stage_scripts
fi

# Wait for cloud-init when present.
wait_cloud_init

resolve_machine_profile
echo "Machine profile: ${MACHINE_PROFILE}"

if $LOCAL_MODE; then
  echo "Verifying system package prerequisites (no sudo)..."
  run_remote "install/packages.sh" \
    "TOOL_SKIP_SYSTEM_PACKAGES=1" \
    "TOOL_MACHINE_PROFILE=${MACHINE_PROFILE}"
else
  echo "Running system package installation..."
  run_remote "install/packages.sh" "TOOL_MACHINE_PROFILE=${MACHINE_PROFILE}"
fi

echo "Running tool installation..."
run_remote "install/tools.sh"

echo "Ensuring \$HOME/.local/bin is in PATH..."
run_shell \
  "grep -qF '\$HOME/.local/bin' ~/.bashrc || echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.bashrc"

echo "Ensuring ~/.env is sourced in ~/.bashrc..."
run_shell \
  "grep -qF 'source \"\$HOME/.env\"' ~/.bashrc || echo '[[ -f \"\$HOME/.env\" ]] && set -a && source \"\$HOME/.env\" && set +a' >> ~/.bashrc"

echo "Ensuring direnv hook is enabled in ~/.bashrc..."
run_shell \
  "grep -qF 'direnv hook bash' ~/.bashrc || echo 'command -v direnv >/dev/null 2>&1 && eval \"\$(direnv hook bash)\"' >> ~/.bashrc"

echo "Ensuring ~/.bash_profile sources ~/.bashrc..."
run_shell \
  "grep -qF '.bashrc' ~/.bash_profile 2>/dev/null || echo '[[ -f \"\$HOME/.bashrc\" ]] && source \"\$HOME/.bashrc\"' >> ~/.bash_profile"

echo "Running terminal installation..."
run_remote "install/terminal.sh" "TOOL_MACHINE_PROFILE=${MACHINE_PROFILE}"

echo "Running font installation..."
run_remote "install/fonts.sh" "TOOL_MACHINE_PROFILE=${MACHINE_PROFILE}"

if [[ "$MACHINE_PROFILE" == "desktop" ]]; then
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
else
  echo "Skipping kitty font configuration on server profile."
fi

OPENBAO_ADDR="${OPENBAO_ADDR:-https://openbao.home.butaco.net}"
BAO_USERNAME="${BAO_USERNAME:-homelab}"

if [[ -z "${BAO_TOKEN:-}" ]]; then
  read -rsp "OpenBao password for ${BAO_USERNAME}: " OPENBAO_PASSWORD; echo
fi

# Populate ~/.env from OpenBao.
echo "Fetching env secrets from OpenBao..."
run_openbao_target "secrets/get-env.sh"

# Populate ~/.kube from OpenBao.
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
