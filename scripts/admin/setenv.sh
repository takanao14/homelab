#!/usr/bin/env bash
set -euo pipefail

ENV_FILE="${HOME}/.env"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "Error: ${ENV_FILE} not found. Create it first." >&2
  exit 1
fi

BAO_ADDR="${OPENBAO_ADDR:-https://openbao.home.butaco.net}"
export BAO_ADDR

if ! command -v bao &>/dev/null; then
  echo "Error: bao CLI not found" >&2
  exit 1
fi

BAO_USERNAME="${BAO_USERNAME:-admin}"
read -rsp "OpenBao password for ${BAO_USERNAME}: " _bao_pass; echo
BAO_TOKEN=$(bao login -token-only -method=userpass username="${BAO_USERNAME}" password="${_bao_pass}")
export BAO_TOKEN

echo "Writing secrets to OpenBao..."

# Build key=value args from .env, skipping comments and empty lines.
kv_args=()
keys=()
while IFS= read -r line; do
  [[ "$line" =~ ^[[:space:]]*# ]] && continue
  [[ -z "${line// }" ]] && continue
  if [[ "$line" =~ ^[[:space:]]*(export[[:space:]]+)?([A-Za-z_][A-Za-z0-9_]*)= ]]; then
    keys+=("${BASH_REMATCH[2]}")
  else
    echo "Error: unsupported .env line: ${line}" >&2
    exit 1
  fi
done < "$ENV_FILE"

set -a
# shellcheck source=/dev/null
source "$ENV_FILE"
set +a

for key in "${keys[@]}"; do
  kv_args+=("${key}=${!key-}")
done

bao kv put secret/provision/env "${kv_args[@]}"
echo "  secret/provision/env (${#kv_args[@]} keys)"

echo "Done."
