# MCP servers

The prd app-of-apps deploys Grafana and NetBox MCP into `mcp`. Each HTTPRoute
publishes `/mcp` through `gateway-system/shared-gateway-envoy` on HTTPS.
The wildcard prd certificate covers both hostnames, and prd external-dns owns
records created from HTTPRoutes. Each server requires its own bearer token.

The `k8s-mcp` OpenBao policy grants prd ESO read access to `k8s/mcp/*` only.
Apply the OpenBao configuration and re-register prd ESO first.
`scripts/secrets/admin/seed-mcp.sh` reads four values from
`.env/secrets.sops.env` and writes changed values to `k8s/mcp/grafana` and
`k8s/mcp/netbox`. Run it after the OpenBao policy is applied and before syncing
the Argo CD application. It does not print values. Re-run it after rotation;
ESO refreshes hourly and Reloader restarts the affected Pod.

Codex and Claude Code use the HTTP entries in `.codex/config.toml` and
`.mcp.json`. Restart each client after changing these files. The header helper
reads the encrypted file directly, so no terminal environment inheritance is
needed.

Check `ExternalSecret` readiness, then confirm each HTTPS endpoint returns `401`
without a bearer token and with the other server's token. Initialize both MCP
connections, list tools, query a bounded Loki range, and fetch a known NetBox
object. The stdio launchers remain available for rollback; restore their
registrations if needed. Keep the upstream API tokens active.
