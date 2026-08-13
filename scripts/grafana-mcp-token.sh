#!/usr/bin/env bash
#
# Create or reuse the Grafana MCP service account and issue a token.
# Only the dotenv assignment is written to stdout; diagnostics use stderr.
#
# Admin auth is resolved in this order:
#   1. GRAFANA_ADMIN_USER / GRAFANA_ADMIN_PASSWORD environment variables
#   2. The in-cluster grafana-admin secret using an explicit context
#
# Requires curl, jq, and kubectl when using in-cluster credentials.
#
# Usage:
#   ./scripts/grafana-mcp-token.sh          # print the assignment line
#
# Replace the assignment through `sops edit`; do not re-encrypt the existing file:
#
#   ./scripts/grafana-mcp-token.sh
#   sops edit .env/secrets.sops.env
#   direnv allow
#
set -euo pipefail

GRAFANA_URL="${GRAFANA_URL:-https://grafana.prd.butaco.net}"
KUBE_CONTEXT="${GRAFANA_KUBE_CONTEXT:-prd-homelab}"
SA_NAME="${GRAFANA_MCP_SA_NAME:-mcp-grafana}"
SA_ROLE="${GRAFANA_MCP_SA_ROLE:-Viewer}"
TOKEN_NAME="${GRAFANA_MCP_TOKEN_NAME:-mcp-grafana-$(date +%Y%m%d-%H%M%S)}"

for bin in curl jq; do
  command -v "${bin}" >/dev/null 2>&1 || { echo "missing dependency: ${bin}" >&2; exit 127; }
done

admin_user="${GRAFANA_ADMIN_USER:-}"
admin_pass="${GRAFANA_ADMIN_PASSWORD:-}"
if [ -z "${admin_user}" ] || [ -z "${admin_pass}" ]; then
  echo "Resolving admin credentials from the grafana-admin secret (context=${KUBE_CONTEXT}, ns=monitoring)..." >&2
  kubectl --context "${KUBE_CONTEXT}" -n monitoring get secret grafana-admin >/dev/null
  admin_user="$(kubectl --context "${KUBE_CONTEXT}" -n monitoring get secret grafana-admin -o jsonpath='{.data.admin-user}' | base64 -d)"
  admin_pass="$(kubectl --context "${KUBE_CONTEXT}" -n monitoring get secret grafana-admin -o jsonpath='{.data.admin-password}' | base64 -d)"
fi

auth=(-u "${admin_user}:${admin_pass}")
api() { curl -fsSk "${auth[@]}" -H 'Content-Type: application/json' "$@"; }

# Find or create the service account.
sa_id="$(api "${GRAFANA_URL}/api/serviceaccounts/search?query=${SA_NAME}" \
  | jq -r --arg n "${SA_NAME}" '.serviceAccounts[]? | select(.name==$n) | .id' | head -n1)"

if [ -z "${sa_id}" ]; then
  echo "Creating service account '${SA_NAME}' (role=${SA_ROLE})..." >&2
  sa_id="$(api -X POST "${GRAFANA_URL}/api/serviceaccounts" \
    -d "$(jq -nc --arg n "${SA_NAME}" --arg r "${SA_ROLE}" '{name:$n, role:$r, isDisabled:false}')" \
    | jq -r '.id')"
else
  echo "Reusing existing service account '${SA_NAME}' (id=${sa_id})." >&2
fi

# Issue a token; Grafana returns its value only once.
token="$(api -X POST "${GRAFANA_URL}/api/serviceaccounts/${sa_id}/tokens" \
  -d "$(jq -nc --arg n "${TOKEN_NAME}" '{name:$n}')" | jq -r '.key')"

[ -n "${token}" ] && [ "${token}" != "null" ] || { echo "failed to obtain token" >&2; exit 1; }

echo "Service account token '${TOKEN_NAME}' created." >&2
echo "GRAFANA_SERVICE_ACCOUNT_TOKEN=\"${token}\""
