#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_dir="$(cd "${script_dir}/../../.." && pwd)"
# shellcheck disable=SC1091
source "${script_dir}/../../lib/openbao-auth.sh"

BAO_ADDR="${OPENBAO_ADDR:-https://openbao.home.butaco.net}"
BAO_USERNAME="${BAO_USERNAME:-admin}"
: "${SOPS_AGE_KEY_FILE:=${HOME}/.config/sops/age/keys.txt}"
export BAO_ADDR SOPS_AGE_KEY_FILE
openbao_authenticate

exec sops exec-env "${repo_dir}/.env/secrets.sops.env" \
  "python3 ${script_dir}/seed-mcp.py"
