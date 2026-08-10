#!/usr/bin/env bash
#
# Launcher for the official NetBox Labs MCP server over stdio.
#
# Runtime selection and credential resolution are shared with grafana-mcp.sh
# through lib/mcp-launcher.sh.
#
# Credentials (NETBOX_URL, NETBOX_TOKEN):
#   - Used directly when already exported (for example by direnv).
#   - Otherwise resolved from the SOPS-encrypted .env/secrets.sops.env so MCP
#     clients never need to store the token in their own configuration.
#
# Image reference:
#   - Pinned to a release tag for reproducibility, bumped in place by Renovate.
#     The published image carries the upstream uv.lock, so transitive
#     dependencies are fixed too — a `uvx --from git+...@tag` install pins only
#     the tag and re-resolves everything below it.
#   - Fully qualified on purpose. Docker resolves a bare name to Docker Hub, but
#     Podman enforces short-name resolution and defines no unqualified-search
#     registries, so the bare name fails on Linux.
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

# stdio transport: keep stdin open (-i), never allocate a TTY (-t breaks framing).
# Only env var names are passed (-e NAME), so values are forwarded from the host
# without ever appearing on the command line.
exec "${runtime}" run -i --rm \
  -e NETBOX_URL \
  -e NETBOX_TOKEN \
  -e VERIFY_SSL \
  -e ENABLE_PLUGIN_DISCOVERY \
  "${NETBOX_MCP_IMAGE:-docker.io/netboxlabs/netbox-mcp-server:${netbox_mcp_version}}"
