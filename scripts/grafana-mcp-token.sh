#!/usr/bin/env bash
#
# Idempotently create the Grafana service account used by the MCP server and
# issue a token. The token (returned only once by Grafana) is printed to stdout
# as a ready-to-encrypt export line for .env/secrets.enc.env; all logs go to
# stderr so the output can be redirected cleanly.
#
# Admin auth is resolved in this order:
#   1. GRAFANA_ADMIN_USER / GRAFANA_ADMIN_PASSWORD environment variables
#   2. The in-cluster `grafana-admin` secret, read via
#      `kubectl --context "${GRAFANA_KUBE_CONTEXT:-prd-homelab}"` so the result
#      does not depend on whatever context happens to be currently selected.
#
# Requirements: curl, jq, and (for option 2) kubectl with access to the prd cluster.
#
# Usage:
#   ./scripts/grafana-mcp-token.sh                       # print export line
#   ./scripts/grafana-mcp-token.sh >> .env/secrets.enc.env && \
#     sops --encrypt --in-place .env/secrets.enc.env     # store encrypted
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

# 1. Find or create the service account (idempotent on name).
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

# 2. Issue a token (the secret value is only returned at creation time).
token="$(api -X POST "${GRAFANA_URL}/api/serviceaccounts/${sa_id}/tokens" \
  -d "$(jq -nc --arg n "${TOKEN_NAME}" '{name:$n}')" | jq -r '.key')"

[ -n "${token}" ] && [ "${token}" != "null" ] || { echo "failed to obtain token" >&2; exit 1; }

echo "Service account token '${TOKEN_NAME}' created." >&2
echo "export GRAFANA_SERVICE_ACCOUNT_TOKEN=\"${token}\""
