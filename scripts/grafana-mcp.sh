#!/usr/bin/env bash
#
# Launcher for the Grafana MCP server (grafana/mcp-grafana) over stdio.
#
# Runtime selection and credential resolution are shared with netbox-mcp.sh
# through lib/mcp-launcher.sh. Override the runtime with GRAFANA_MCP_RUNTIME
# (e.g. "docker" or "podman") when needed.
#
# Image reference:
#   - The vendor's own `grafana/mcp-grafana`, not Docker's curated `mcp/grafana`
#     mirror: the latter publishes only a `latest` tag, so it cannot be pinned
#     or tracked by Renovate.
#   - Pinned to a release tag, bumped in place by Renovate.
#   - Fully qualified on purpose. Docker resolves a bare name to Docker Hub, but
#     Podman enforces short-name resolution and defines no unqualified-search
#     registries, so the bare name fails on Linux.
#
# Tool exposure:
#   - Defaults to the read-only observability categories needed by local agents.
#   - Write tools are disabled independently of the Grafana Viewer token.
#   - Override GRAFANA_MCP_ENABLED_TOOLS only when another category is required.
#
# Credentials (GRAFANA_URL, GRAFANA_SERVICE_ACCOUNT_TOKEN):
#   - Used directly if already exported (e.g. Claude Code launched under direnv).
#   - Otherwise self-resolved from the SOPS-encrypted .env/secrets.sops.env, so the
#     launcher works from any MCP client (Codex, Cursor, …) regardless of cwd or
#     whether direnv has loaded. Secrets are never hardcoded here.
#
set -euo pipefail

# Derive the repo root from this script's location so cwd does not matter.
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source-path=SCRIPTDIR source=lib/mcp-launcher.sh
source "${script_dir}/lib/mcp-launcher.sh"

# An empty token would leave the server running against Grafana with a 401.
mcp_load_sops_env GRAFANA_SERVICE_ACCOUNT_TOKEN

: "${GRAFANA_URL:=https://grafana.prd.butaco.net}"
export GRAFANA_URL GRAFANA_SERVICE_ACCOUNT_TOKEN

runtime="$(mcp_resolve_container_runtime "${GRAFANA_MCP_RUNTIME:-}")" || exit $?

# stdio transport: keep stdin open (-i), never allocate a TTY (-t breaks framing).
# Only env var names are passed (-e NAME), so values are forwarded from the host
# without ever appearing on the command line.
enabled_tools="${GRAFANA_MCP_ENABLED_TOOLS:-search,datasource,prometheus,loki,dashboard,navigation}"
max_loki_log_limit="${GRAFANA_MCP_MAX_LOKI_LOG_LIMIT:-20}"
server_args=(
  -t stdio
  --enabled-tools "${enabled_tools}"
  --max-loki-log-limit "${max_loki_log_limit}"
)
if [ "${GRAFANA_MCP_DISABLE_WRITE:-true}" = "true" ]; then
  server_args+=(--disable-write)
fi

# renovate: datasource=docker depName=grafana/mcp-grafana
grafana_mcp_version="${GRAFANA_MCP_VERSION:-1.1.0}"

exec "${runtime}" run -i --rm \
  -e GRAFANA_URL \
  -e GRAFANA_SERVICE_ACCOUNT_TOKEN \
  "${GRAFANA_MCP_IMAGE:-docker.io/grafana/mcp-grafana:${grafana_mcp_version}}" "${server_args[@]}"
