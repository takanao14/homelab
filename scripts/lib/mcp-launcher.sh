#!/usr/bin/env bash

# Shared plumbing for the MCP server launchers (grafana-mcp.sh, netbox-mcp.sh).
#
# MCP clients (Claude Code, Codex, Cursor, …) start these launchers with an
# arbitrary cwd and without the user's interactive shell environment, so each
# launcher resolves its own credentials and container runtime instead of relying
# on direnv or a warm shell.

# Diagnostic prefix, taken from the sourcing launcher's filename (netbox-mcp.sh
# -> "netbox-mcp"). Callers may set MCP_LOG_PREFIX beforehand to override it.
: "${MCP_LOG_PREFIX:=$(basename "${0}" .sh)}"

_mcp_lib_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

mcp_warn() {
  echo "${MCP_LOG_PREFIX}: $*" >&2
}

# mcp_load_sops_env <guard_var_name>
#
# Decrypts .env/secrets.sops.env into the environment, unless <guard_var_name>
# is already set — an environment supplied by direnv always wins.
mcp_load_sops_env() {
  local guard_var="$1"

  if [ -n "${!guard_var:-}" ]; then
    return 0
  fi

  local secret_file="${_mcp_lib_dir}/../../.env/secrets.sops.env"

  # Point sops at the age key explicitly. On macOS sops otherwise defaults to
  # ~/Library/Application Support/sops/age/keys.txt (Go's os.UserConfigDir), so
  # clients that do not inherit SOPS_AGE_KEY_FILE from the shell profile would
  # fail to decrypt and the server would start with an empty credential.
  : "${SOPS_AGE_KEY_FILE:=${XDG_CONFIG_HOME:-${HOME}/.config}/sops/age/keys.txt}"
  export SOPS_AGE_KEY_FILE

  if [ -f "${secret_file}" ] && command -v sops >/dev/null 2>&1; then
    eval "$(sops --decrypt "${secret_file}")"
  fi
}

# mcp_resolve_container_runtime [override]
#
# Echoes the container runtime to use:
#   macOS -> OrbStack's drop-in docker CLI when installed, otherwise Podman
#   Linux -> Podman
# Returns non-zero with actionable diagnostics when the runtime is missing or
# its macOS virtual machine is not ready.
mcp_resolve_container_runtime() {
  local runtime="${1:-}"

  if [ -z "${runtime}" ]; then
    case "$(uname -s)" in
      Darwin)
        if command -v orb >/dev/null 2>&1 || [ -d "/Applications/OrbStack.app" ]; then
          runtime="docker"  # OrbStack provides a drop-in docker CLI
        else
          runtime="podman"
        fi
        ;;
      *) runtime="podman" ;;
    esac
  fi

  if ! command -v "${runtime}" >/dev/null 2>&1; then
    if [ "${runtime}" = "podman" ] && [ "$(uname -s)" = "Darwin" ]; then
      mcp_warn "OrbStack was not detected and Podman is not found in PATH."
      mcp_warn "Install Podman from the official macOS installer, then run 'podman machine init --now'."
    else
      mcp_warn "container runtime '${runtime}' not found in PATH"
    fi
    return 127
  fi

  if [ "${runtime}" = "podman" ] && [ "$(uname -s)" = "Darwin" ]; then
    local podman_info_error
    if ! podman_info_error="$("${runtime}" info 2>&1 >/dev/null)"; then
      if printf '%s\n' "${podman_info_error}" | grep -qi "krunkit"; then
        mcp_warn "Podman on macOS cannot find the 'krunkit' binary."
        mcp_warn "Install Podman from the official macOS installer, or install/configure krunkit manually."
      else
        mcp_warn "Podman is installed, but the macOS Podman machine is not ready."
        mcp_warn "Try 'podman machine init --now' or 'podman machine start'."
        printf '%s\n' "${podman_info_error}" >&2
      fi
      return 1
    fi
  fi

  printf '%s\n' "${runtime}"
}
