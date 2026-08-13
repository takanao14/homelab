#!/usr/bin/env bash
#
# Print a NetBox v2 MCP token as a dotenv assignment for the SOPS environment.
# The NetBox role installs or rotates the matching read-only token.
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
