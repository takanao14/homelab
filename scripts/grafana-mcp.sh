#!/usr/bin/env bash
#
# Launcher for the Grafana MCP server (grafana/mcp-grafana) over stdio.
#
# Runtime selection:
#   - macOS (Darwin) -> OrbStack, invoked via the `docker` CLI
#   - Linux          -> Podman, invoked via the `podman` CLI
# Override with GRAFANA_MCP_RUNTIME (e.g. "docker" or "podman") when needed.
#
# Credentials (GRAFANA_URL, GRAFANA_SERVICE_ACCOUNT_TOKEN):
#   - Used directly if already exported (e.g. Claude Code launched under direnv).
#   - Otherwise self-resolved from the SOPS-encrypted .env/secrets.enc.env, so the
#     launcher works from any MCP client (Codex, Cursor, …) regardless of cwd or
#     whether direnv has loaded. Secrets are never hardcoded here.
#
set -euo pipefail

# Self-resolve credentials when the environment does not already carry them.
# Derive the repo root from this script's location so cwd does not matter.
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [ -z "${GRAFANA_SERVICE_ACCOUNT_TOKEN:-}" ]; then
  secret_file="${script_dir}/../.env/secrets.enc.env"
  # Point sops at the age key explicitly. On macOS sops otherwise defaults to
  # ~/Library/Application Support/sops/age/keys.txt (Go's os.UserConfigDir),
  # so clients that do not inherit SOPS_AGE_KEY_FILE from the shell profile
  # (e.g. Codex/Cursor launching this over stdio) would fail to decrypt and
  # the server would start with an empty token (Grafana 401).
  : "${SOPS_AGE_KEY_FILE:=${XDG_CONFIG_HOME:-${HOME}/.config}/sops/age/keys.txt}"
  export SOPS_AGE_KEY_FILE
  if [ -f "${secret_file}" ] && command -v sops >/dev/null 2>&1; then
    eval "$(sops --decrypt "${secret_file}")"
  fi
fi
: "${GRAFANA_URL:=https://grafana.prd.butaco.net}"
export GRAFANA_URL GRAFANA_SERVICE_ACCOUNT_TOKEN

runtime="${GRAFANA_MCP_RUNTIME:-}"
if [ -z "${runtime}" ]; then
  case "$(uname -s)" in
    Darwin) runtime="docker" ;;  # OrbStack provides a drop-in docker CLI
    *)      runtime="podman" ;;
  esac
fi

if ! command -v "${runtime}" >/dev/null 2>&1; then
  echo "grafana-mcp: container runtime '${runtime}' not found in PATH" >&2
  exit 127
fi

# stdio transport: keep stdin open (-i), never allocate a TTY (-t breaks framing).
# Only env var names are passed (-e NAME), so values are forwarded from the host
# without ever appearing on the command line.
exec "${runtime}" run -i --rm \
  -e GRAFANA_URL \
  -e GRAFANA_SERVICE_ACCOUNT_TOKEN \
  "${GRAFANA_MCP_IMAGE:-mcp/grafana}" -t stdio
