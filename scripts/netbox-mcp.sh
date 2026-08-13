#!/usr/bin/env bash
#
# NetBox MCP stdio launcher. Shared setup lives in lib/mcp-launcher.sh.
#
# Credentials (NETBOX_URL, NETBOX_TOKEN):
#   - Use exported values, otherwise load the SOPS-encrypted environment.
#
# Image reference:
#   - Pinned with upstream uv.lock and tracked by Renovate.
#   - Fully qualified for Podman short-name compatibility.
#
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source-path=SCRIPTDIR source=lib/mcp-launcher.sh
source "${script_dir}/lib/mcp-launcher.sh"

mcp_load_sops_env NETBOX_TOKEN

: "${NETBOX_URL:=https://netbox-ui.home.butaco.net/}"
: "${VERIFY_SSL:=true}"
: "${ENABLE_PLUGIN_DISCOVERY:=false}"

if [ -z "${NETBOX_TOKEN:-}" ]; then
  mcp_warn "NETBOX_TOKEN is not set."
  mcp_warn "Add a read-only NetBox API token to .env/secrets.sops.env."
  exit 1
fi

runtime="$(mcp_resolve_container_runtime "${NETBOX_MCP_RUNTIME:-}")" || exit $?

export NETBOX_URL NETBOX_TOKEN VERIFY_SSL ENABLE_PLUGIN_DISCOVERY

# renovate: datasource=docker depName=netboxlabs/netbox-mcp-server
netbox_mcp_version="${NETBOX_MCP_VERSION:-1.2.1}"

# Keep stdin open without a TTY; pass only environment variable names.
exec "${runtime}" run -i --rm \
  -e NETBOX_URL \
  -e NETBOX_TOKEN \
  -e VERIFY_SSL \
  -e ENABLE_PLUGIN_DISCOVERY \
  "${NETBOX_MCP_IMAGE:-docker.io/netboxlabs/netbox-mcp-server:${netbox_mcp_version}}"
