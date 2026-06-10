#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/../../lib/openbao-auth.sh"

ENV_FILE="${HOME}/.env"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "Error: ${ENV_FILE} not found. Create it first." >&2
  exit 1
fi

BAO_ADDR="${OPENBAO_ADDR:-https://openbao.home.butaco.net}"
export BAO_ADDR

for command in bao jq; do
  if ! command -v "$command" &>/dev/null; then
    echo "Error: ${command} CLI not found" >&2
    exit 1
  fi
done

BAO_USERNAME="${BAO_USERNAME:-admin}"
openbao_authenticate

echo "Writing secrets to OpenBao..."

# Parse values as data without expanding shell variables or executing commands.
parse_env_value() {
  local raw="$1"
  local inner
  local escaped_apostrophe="'\\''"
  local apostrophe="'"

  if [[ "$raw" == \'* ]]; then
    if [[ "$raw" != *\' ]]; then
      return 1
    fi
    inner="${raw:1:${#raw}-2}"
    REPLY="${inner//"$escaped_apostrophe"/"$apostrophe"}"
  elif [[ "$raw" == \"* ]]; then
    if [[ "$raw" != *\" ]]; then
      return 1
    fi
    if ! REPLY="$(jq -Rr 'fromjson' <<<"$raw")"; then
      return 1
    fi
  else
    REPLY="$raw"
  fi
}

# Build key=value args from .env, skipping comments and empty lines.
kv_args=()
while IFS= read -r line || [[ -n "$line" ]]; do
  [[ "$line" =~ ^[[:space:]]*# ]] && continue
  [[ "$line" =~ ^[[:space:]]*$ ]] && continue
  if [[ "$line" =~ ^[[:space:]]*(export[[:space:]]+)?([A-Za-z_][A-Za-z0-9_]*)=(.*)$ ]]; then
    key="${BASH_REMATCH[2]}"
    if [[ "$key" == BAO_TOKEN || "$key" == BAO_PASSWORD ]]; then
      echo "Warning: skipping reserved OpenBao variable ${key}" >&2
      continue
    fi
    if ! parse_env_value "${BASH_REMATCH[3]}"; then
      echo "Error: unsupported .env value for ${key}" >&2
      exit 1
    fi
    kv_args+=("${key}=${REPLY}")
  else
    echo "Error: unsupported .env line: ${line}" >&2
    exit 1
  fi
done < "$ENV_FILE"

bao kv put secret/provision/env "${kv_args[@]}"
echo "  secret/provision/env (${#kv_args[@]} keys)"

echo "Done."
