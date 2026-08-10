#!/usr/bin/env bash
#
# Generate the desired NetBox v2 API token for the MCP identity.
#
# This script only prints a dotenv assignment. Store it in the existing
# SOPS-encrypted .env/secrets.sops.env, load it with direnv, and then run the
# NetBox Ansible playbook. The role creates or rotates the matching read-only
# token without exposing the plaintext in Ansible output.
#
set -euo pipefail

command -v python3 >/dev/null 2>&1 || {
  echo "netbox-mcp-token: python3 is required" >&2
  exit 127
}

python3 - <<'PY'
import secrets
import string

alphabet = string.ascii_letters + string.digits
key = "".join(secrets.choice(alphabet) for _ in range(12))
secret = "".join(secrets.choice(alphabet) for _ in range(40))
print(f'NETBOX_TOKEN="nbt_{key}.{secret}"')
PY
