#!/usr/bin/env bash
#
# HTTP MCP headers helper for .mcp.json and .codex/config.toml.
#
# Credentials:
#   - The token is emitted only through the printf builtin, never argv.
#
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MCP_LOG_PREFIX="mcp-http-headers"
# shellcheck source-path=SCRIPTDIR source=lib/mcp-launcher.sh
source "${script_dir}/lib/mcp-launcher.sh"

case "${1:-}" in
  grafana) token_var=MCP_GRAFANA_SERVER_TOKEN ;;
  netbox) token_var=MCP_NETBOX_SERVER_TOKEN ;;
  *)
    mcp_warn "usage: mcp-http-headers.sh grafana|netbox"
    exit 2
    ;;
esac

mcp_load_sops_env "${token_var}"
if [ -z "${!token_var:-}" ]; then
  mcp_warn "${token_var} is not set."
  exit 1
fi

# Restricting the charset keeps the hand-built JSON valid without escaping.
if [[ ! "${!token_var}" =~ ^[A-Za-z0-9._~+/=-]+$ ]]; then
  mcp_warn "${token_var} contains unexpected characters."
  exit 1
fi

printf '{"Authorization": "Bearer %s"}\n' "${!token_var}"
