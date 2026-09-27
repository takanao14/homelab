# MCP servers

The prd app-of-apps deploys Grafana and NetBox MCP into `mcp`. Each HTTPRoute
publishes `/mcp` through `gateway-system/shared-gateway-envoy` on HTTPS.
The Gateway policy allows `192.168.10.0/24`; verify the source IP seen by Envoy
before adding a VPN range. The wildcard prd certificate covers both hostnames,
and prd external-dns owns records created from HTTPRoutes.

The `k8s-mcp` OpenBao policy grants prd ESO read access to `k8s/mcp/*` only.
Apply the OpenBao configuration and re-register prd ESO first.
`scripts/secrets/admin/seed-mcp.sh` reads four values from
`.env/secrets.sops.env` and writes changed values to `k8s/mcp/grafana` and
`k8s/mcp/netbox`. Run it after the OpenBao policy is applied and before syncing
the Argo CD application. It does not print values. Re-run it after rotation;
ESO refreshes hourly and Reloader restarts the affected Pod.

After cluster checks pass, add these entries under separate names while the
stdio registrations remain active, then restart each client.
Use the same entries with `headersHelper` instead of `http_headers_helper` in
Claude Code's user-scope JSON configuration (`type: "http"`). The helper reads
the encrypted file directly, so no terminal environment inheritance is needed.

```toml
[mcp_servers.grafana-cluster]
url = "https://mcp-grafana.prd.butaco.net/mcp"
http_headers_helper = "/usr/bin/python3 /Users/takanao/lab/homelab/scripts/mcp-http-headers.py grafana"

[mcp_servers.netbox-cluster]
url = "https://mcp-netbox.prd.butaco.net/mcp"
http_headers_helper = "/usr/bin/python3 /Users/takanao/lab/homelab/scripts/mcp-http-headers.py netbox"
```

```json
{
  "mcpServers": {
    "grafana-cluster": {
      "type": "http",
      "url": "https://mcp-grafana.prd.butaco.net/mcp",
      "headersHelper": "/usr/bin/python3 /Users/takanao/lab/homelab/scripts/mcp-http-headers.py grafana"
    },
    "netbox-cluster": {
      "type": "http",
      "url": "https://mcp-netbox.prd.butaco.net/mcp",
      "headersHelper": "/usr/bin/python3 /Users/takanao/lab/homelab/scripts/mcp-http-headers.py netbox"
    }
  }
}
```

Check `ExternalSecret` readiness, then confirm each HTTPS endpoint returns `401`
without a bearer token and with the other server's token. From an allowed
client, initialize both MCP connections, list tools, query a bounded Loki range,
and fetch a known NetBox object. Confirm disallowed source ranges cannot access
the routes. Retain the stdio launchers until both desktop clients work after
restart. Roll back by restoring their stdio registrations and disabling the two
HTTPRoutes; keep the upstream API tokens active.
