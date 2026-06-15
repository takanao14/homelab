#!/usr/bin/env bash
#
# Launcher for the Grafana MCP server (grafana/mcp-grafana) over stdio.
#
# Runtime selection:
#   - macOS (Darwin) -> OrbStack, invoked via the `docker` CLI
#   - Linux          -> Podman, invoked via the `podman` CLI
# Override with GRAFANA_MCP_RUNTIME (e.g. "docker" or "podman") when needed.
#
# Required environment (injected via .envrc / direnv, never hardcoded):
#   - GRAFANA_URL                     e.g. https://grafana.prd.butaco.net
#   - GRAFANA_SERVICE_ACCOUNT_TOKEN   Grafana service account token
#
set -euo pipefail

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
