#!/usr/bin/env bash
#
# stdio bridge to cluster-hosted HTTP MCP for clients that cannot run the
# .mcp.json header helper. Requires mcp-proxy (brew install mcp-proxy).
#
# Credentials:
#   - The caller token reaches mcp-proxy only as API_ACCESS_TOKEN, never argv.
#
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MCP_LOG_PREFIX="mcp-http-proxy"
# shellcheck source-path=SCRIPTDIR source=lib/mcp-launcher.sh
source "${script_dir}/lib/mcp-launcher.sh"

case "${1:-}" in
  grafana) token_var=MCP_GRAFANA_SERVER_TOKEN ;;
  netbox) token_var=MCP_NETBOX_SERVER_TOKEN ;;
  *)
    mcp_warn "usage: mcp-http-proxy.sh grafana|netbox"
    exit 2
    ;;
esac

if ! command -v mcp-proxy >/dev/null 2>&1; then
  mcp_warn "mcp-proxy not found in PATH; install it with 'brew install mcp-proxy'."
  exit 127
fi

url="$(python3 -I -c 'import json, sys; print(json.load(open(sys.argv[1]))["mcpServers"][sys.argv[2]]["url"])' \
  "${script_dir}/../.mcp.json" "$1")"

mcp_load_sops_env "${token_var}"
if [ -z "${!token_var:-}" ]; then
  mcp_warn "${token_var} is not set."
  exit 1
fi

API_ACCESS_TOKEN="${!token_var}"
export API_ACCESS_TOKEN
exec mcp-proxy --transport streamablehttp "${url}"
