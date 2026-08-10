#!/usr/bin/env bash
#
# Launcher for the official NetBox Labs MCP server over stdio.
#
# Credentials (NETBOX_URL, NETBOX_TOKEN):
#   - Used directly when already exported (for example by direnv).
#   - Otherwise resolved from the SOPS-encrypted .env/secrets.sops.env so MCP
#     clients never need to store the token in their own configuration.
#
# The upstream source is pinned to a release tag for reproducibility. Override
# NETBOX_MCP_SOURCE only while deliberately testing another version.
#
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [ -z "${NETBOX_TOKEN:-}" ]; then
  secret_file="${script_dir}/../.env/secrets.sops.env"
  : "${SOPS_AGE_KEY_FILE:=${XDG_CONFIG_HOME:-${HOME}/.config}/sops/age/keys.txt}"
  export SOPS_AGE_KEY_FILE

  if [ -f "${secret_file}" ] && command -v sops >/dev/null 2>&1; then
    eval "$(sops --decrypt "${secret_file}")"
  fi
fi

: "${NETBOX_URL:=https://netbox-ui.home.butaco.net/}"
: "${VERIFY_SSL:=true}"
: "${ENABLE_PLUGIN_DISCOVERY:=false}"

if [ -z "${NETBOX_TOKEN:-}" ]; then
  echo "netbox-mcp: NETBOX_TOKEN is not set." >&2
  echo "netbox-mcp: Add a read-only NetBox API token to .env/secrets.sops.env." >&2
  exit 1
fi

if ! command -v uvx >/dev/null 2>&1; then
  echo "netbox-mcp: uvx is not found in PATH; install uv first." >&2
  exit 127
fi

export NETBOX_URL NETBOX_TOKEN VERIFY_SSL ENABLE_PLUGIN_DISCOVERY
export UV_NO_PROGRESS=1

source_ref="${NETBOX_MCP_SOURCE:-git+https://github.com/netboxlabs/netbox-mcp-server.git@v1.2.1}"
exec uvx --from "${source_ref}" netbox-mcp-server
