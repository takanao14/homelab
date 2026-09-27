#!/usr/bin/env python3
import json
import os
import subprocess
import sys
from pathlib import Path


TOKENS = {
    "grafana": "MCP_GRAFANA_SERVER_TOKEN",
    "netbox": "MCP_NETBOX_SERVER_TOKEN",
}
SOPS = Path("/opt/homebrew/bin/sops")
SECRET_FILE = Path(__file__).resolve().parent.parent / ".env/secrets.sops.env"


def main():
    if len(sys.argv) not in (2, 3) or sys.argv[1] not in TOKENS:
        raise ValueError("Select grafana or netbox")
    server = sys.argv[1]
    if not SOPS.is_file() or not SECRET_FILE.is_file():
        raise RuntimeError("SOPS or encrypted credential file is unavailable")
    env = os.environ.copy()
    env["SOPS_AGE_KEY_FILE"] = str(Path.home() / ".config/sops/age/keys.txt")
    if len(sys.argv) == 3:
        if sys.argv[2] != "--emit":
            raise ValueError("Invalid helper invocation")
        token = env.get(TOKENS[server])
        if not token:
            raise RuntimeError("MCP credential is unavailable")
        print(json.dumps({"Authorization": f"Bearer {token}"}))
        return
    result = subprocess.run(
        [str(SOPS), "exec-env", str(SECRET_FILE),
         f"{sys.executable} {Path(__file__).resolve()} {server} --emit"],
        env=env,
        check=False,
    )
    if result.returncode:
        raise RuntimeError("MCP credential could not be decrypted")


if __name__ == "__main__":
    try:
        main()
    except (RuntimeError, ValueError) as error:
        print(f"mcp-http-headers: {error}", file=sys.stderr)
        sys.exit(1)
