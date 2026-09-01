#!/usr/bin/env bash
#
# Grafana MCP stdio launcher. Shared setup lives in lib/mcp-launcher.sh.
# Override the runtime with GRAFANA_MCP_RUNTIME.
#
# Image reference:
#   - Vendor image, pinned and tracked by Renovate; the curated mirror has only latest.
#   - Fully qualified for Podman short-name compatibility.
#
# Tool exposure:
#   - Read-only observability and alerting categories by default; write tools are disabled.
#   - Override GRAFANA_MCP_ENABLED_TOOLS when needed.
#
# Credentials (GRAFANA_URL, GRAFANA_SERVICE_ACCOUNT_TOKEN):
#   - Use exported values, otherwise load the SOPS-encrypted environment.
#
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source-path=SCRIPTDIR source=lib/mcp-launcher.sh
source "${script_dir}/lib/mcp-launcher.sh"

# An empty token would leave the server running against Grafana with a 401.
mcp_load_sops_env GRAFANA_SERVICE_ACCOUNT_TOKEN

: "${GRAFANA_URL:=https://grafana.prd.butaco.net}"
export GRAFANA_URL GRAFANA_SERVICE_ACCOUNT_TOKEN

runtime="$(mcp_resolve_container_runtime "${GRAFANA_MCP_RUNTIME:-}")" || exit $?

# Keep stdin open without a TTY; pass only environment variable names.
enabled_tools="${GRAFANA_MCP_ENABLED_TOOLS:-search,datasource,prometheus,loki,dashboard,navigation,alerting}"
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
grafana_mcp_version="${GRAFANA_MCP_VERSION:-1.3.0}"

exec "${runtime}" run -i --rm \
  -e GRAFANA_URL \
  -e GRAFANA_SERVICE_ACCOUNT_TOKEN \
  "${GRAFANA_MCP_IMAGE:-docker.io/grafana/mcp-grafana:${grafana_mcp_version}}" "${server_args[@]}"
