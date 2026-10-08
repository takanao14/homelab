# ADR-0057: Host authenticated MCP servers in the prd cluster

- **Status:** Accepted
- **Amended by:** [ADR-0060](0060-mcp-relies-on-bearer-authentication.md)
- **Date:** 2026-09-27
- **Related:** [ADR-0014](0014-argocd-app-of-apps-shared-helm-chart.md), [ADR-0026](0026-eso-over-helm-secrets-for-in-cluster-secrets.md), [ADR-0044](0044-external-dns-syncs-owned-record-lifecycle.md)

## Context

Grafana and NetBox MCP currently run as workstation stdio containers. Their
upstream API credentials live in the workstation's SOPS file, so each client
needs a local container runtime and access to those credentials.

## Decision

Run one read-only HTTP MCP server for each upstream in the prd cluster's `mcp`
namespace. Expose only `/mcp` over the shared HTTPS Gateway, with a source CIDR
policy and pod ingress limited to that Gateway. Keep Grafana in `monitoring` and
NetBox on its VM. This creates a deliberate dependency on the prd cluster.

Keep four distinct credentials: one upstream token and one caller bearer token
per server. The SOPS-encrypted `.env/secrets.sops.env` file is the durable source;
an operator seeds dedicated OpenBao paths, and ESO supplies separate Kubernetes
Secrets. The prd ESO role reads only the MCP prefix through its `k8s-mcp` policy.
Clients receive only their own caller token through a local header helper.

Keep the stdio launchers as outage fallback. Switch active client registrations
only after both authenticated HTTPS connections pass acceptance checks.

## Consequences

- A prd cluster or Gateway outage interrupts remote MCP clients; the stdio
  launchers remain available for recovery.
- Rotating a SOPS token requires reseeding OpenBao; ESO and Reloader then update
  the corresponding Pod. Caller tokens must be changed in clients at the same
  time.
