#!/usr/bin/env python3
import json
import os
import sys
import urllib.error
import urllib.request


def request(path, method="GET", data=None):
    base = os.environ["BAO_ADDR"].rstrip("/")
    payload = None if data is None else json.dumps({"data": data}).encode()
    req = urllib.request.Request(
        f"{base}/v1/secret/data/{path}",
        data=payload,
        method=method,
        headers={"X-Vault-Token": os.environ["BAO_TOKEN"], "Content-Type": "application/json"},
    )
    try:
        with urllib.request.urlopen(req, timeout=15) as response:
            return json.load(response)
    except urllib.error.HTTPError as error:
        if method == "GET" and error.code == 404:
            return None
        raise RuntimeError(f"OpenBao {method} {path} failed with HTTP {error.code}") from None


def main():
    entries = {
        "grafana": ("GRAFANA_SERVICE_ACCOUNT_TOKEN", "MCP_GRAFANA_SERVER_TOKEN"),
        "netbox": ("NETBOX_TOKEN", "MCP_NETBOX_SERVER_TOKEN"),
    }
    for name, (upstream_name, caller_name) in entries.items():
        upstream = os.environ.get(upstream_name)
        caller = os.environ.get(caller_name)
        if not upstream or not caller or upstream == caller:
            raise RuntimeError(f"Missing or reused MCP credential for {name}")
        path = f"k8s/mcp/{name}"
        desired = {"upstream-token": upstream, "caller-token": caller}
        current = request(path)
        if current is not None and current["data"]["data"] == desired:
            print(f"{path}: unchanged")
            continue
        request(path, "POST", desired)
        print(f"{path}: updated")


if __name__ == "__main__":
    try:
        main()
    except (KeyError, RuntimeError, urllib.error.URLError) as error:
        print(f"MCP seed failed: {error}", file=sys.stderr)
        sys.exit(1)
