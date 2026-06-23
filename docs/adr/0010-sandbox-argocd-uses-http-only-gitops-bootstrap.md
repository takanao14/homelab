# ADR-0010: Sandbox Argo CD uses HTTP-only GitOps bootstrap without cert-manager

- **Status:** Accepted
- **Date:** 2026-06-24
- **Related:** [`k8s/argocd/README.md`](../../k8s/argocd/README.md), [`k8s/README.md`](../../k8s/README.md), [`k8s/gateway/README.md`](../../k8s/gateway/README.md), [`k8s/externalDNS/README.md`](../../k8s/externalDNS/README.md), [`k8s/eso/README.md`](../../k8s/eso/README.md)

## Context

The sandbox cluster is used for short-lived experimentation and storage
validation. It should be bootstrapped through the same Argo CD App of Apps
pattern as dev and prd, but it intentionally has a smaller platform footprint.

The dev and prd clusters expose services through the shared Cilium Gateway with
HTTPS termination, wildcard certificates, cert-manager, and Cloudflare DNS-01.
For sandbox, that is more machinery than needed. The cluster is reachable only
from the trusted internal network, and avoiding cert-manager keeps the first
bootstrap loop simpler.

## Decision

Create a dedicated `k8s/argocd/sandbox` environment that bootstraps Argo CD and
then manages a small sandbox platform set:

- `argocd`
- `gateway`
- `external-secrets`
- `external-dns`
- sandbox-specific workloads added intentionally, such as `longhorn-ui`

Sandbox uses the same shared charts as dev and prd, but environment values
select HTTP-only behavior:

- the shared Gateway renders the `http` listener and does not render the
  `https` listener;
- sandbox HTTPRoutes bind to `sectionName: http`;
- Argo CD runs with `server.extraArgs: ["--insecure"]` and is reached at
  `http://argocd.sandbox.butaco.net`;
- no `cert-manager` or `cert-manager-config` Application is present in the
  sandbox App of Apps tree.

Secrets still use External Secrets Operator and OpenBao. Sandbox uses its own
OpenBao Kubernetes auth mount, `kubernetes-sandbox`, so policy can be scoped
separately from dev and prd. DNS records are created by external-dns from
Gateway API `HTTPRoute` resources and limited to `sandbox.butaco.net.` with a
dedicated TXT owner ID.

## Alternatives considered

- **Copy dev/prd charts and make sandbox-specific templates** — would avoid
  conditionals, but creates divergent Gateway and Argo CD route logic. *Rejected*
  in favor of shared charts with environment values.
- **Install cert-manager in sandbox and use HTTPS everywhere** — aligns with
  dev/prd, but adds Cloudflare credentials and certificate lifecycle to a
  short-lived internal cluster. *Rejected for sandbox.*
- **Skip Argo CD and apply sandbox manifests manually** — faster initially, but
  loses the same reconciliation model used by dev/prd. *Rejected.*
- **Copy all dev applications into sandbox** — convenient, but expands blast
  radius and resource usage before each app has a sandbox requirement. *Rejected.*

## Consequences

- Sandbox follows the same GitOps control-plane pattern as dev and prd while
  keeping the platform set intentionally small.
- Shared Gateway and Argo CD route charts must keep listener selection
  configurable; dev/prd continue to render HTTPS and sandbox renders HTTP only.
- HTTP carries credentials and sessions without transport encryption. Sandbox
  Gateway addresses must stay on the trusted internal network and must not be
  published through public DNS or an internet-facing router. If that boundary
  changes, sandbox should enable HTTPS before use.
- cert-manager resources, CRDs, issuers, Cloudflare credentials, Certificates,
  and ReferenceGrants are intentionally absent from sandbox.
- OpenBao policy for sandbox can remain narrower than dev/prd and should only
  grow as sandbox applications require secrets.
