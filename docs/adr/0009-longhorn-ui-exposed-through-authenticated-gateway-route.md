# ADR-0009: Longhorn UI is exposed through an authenticated Gateway route

- **Status:** Accepted
- **Date:** 2026-06-24
- **Related:** [`k8s/longhorn-ui/README.md`](../../k8s/longhorn-ui/README.md), [`k8s/argocd/README.md`](../../k8s/argocd/README.md), [`k0s/README.md`](../../k0s/README.md)

## Context

The sandbox cluster uses Longhorn as its storage provider. Longhorn is installed
by the k0s bootstrap Helmfile because storage is cluster infrastructure: it must
exist independently of application GitOps and should not depend on Argo CD being
healthy.

At the same time, the Longhorn UI needs convenient browser access from the LAN.
The sandbox cluster already exposes HTTP-only services through the shared Cilium
Gateway and creates DNS records through external-dns from `HTTPRoute`
resources.

Longhorn's frontend service does not provide a repository-managed authentication
layer suitable for direct exposure. Even in sandbox, exposing
`longhorn-frontend` directly would make a storage management UI too easy to
reach.

## Decision

Keep the Longhorn Helm release in the `k0s/` bootstrap layer, and manage only
the UI exposure layer from Argo CD:

```text
http://longhorn.sandbox.butaco.net
  -> gateway-system/shared-gateway:http
  -> longhorn-system/longhorn-ui-proxy
  -> longhorn-system/longhorn-frontend:80
```

The `k8s/longhorn-ui` chart deploys an unprivileged nginx reverse proxy with
Basic Auth enabled. The htpasswd content is read by External Secrets Operator
from OpenBao at:

```text
secret/k8s/longhorn-ui/basic-auth
```

The sandbox OpenBao Kubernetes auth role includes a narrow `k8s-longhorn-ui`
policy that can read only `secret/k8s/longhorn-ui/*`. The DNS record is created
by external-dns from the `HTTPRoute` hostname.

## Alternatives considered

- **Expose `longhorn-frontend` directly with an HTTPRoute** — simplest and would
  work technically, but leaves the storage administration UI without an explicit
  access control layer. *Rejected.*
- **Move the Longhorn Helm release itself under Argo CD** — makes all Kubernetes
  resources GitOps-managed in one place, but entangles cluster storage bootstrap
  with application GitOps. If Argo CD or its storage dependencies are unhealthy,
  recovery becomes more awkward. *Rejected.*
- **Use cert-manager and HTTPS for sandbox Longhorn UI** — aligns with dev/prd
  public routes, but sandbox intentionally avoids cert-manager and is HTTP-only.
  *Rejected for sandbox.*
- **Use OIDC/OAuth instead of Basic Auth** — stronger and nicer for multi-user
  access, but disproportionate for the current LAN-only sandbox UI. *Deferred.*

## Consequences

- Longhorn storage lifecycle remains owned by k0s bootstrap; the UI route is
  owned by Argo CD.
- Sandbox Longhorn UI access is protected by Basic Auth while preserving the
  cluster's HTTP-only sandbox design.
- OpenBao is the single source of truth for the htpasswd secret; no credentials
  are committed to Git.
- Argo CD drift from Kubernetes API defaulting must be handled explicitly:
  `ExternalSecret.remoteRef` default fields are pinned, and the sandbox
  `longhorn-ui` Application ignores known Gateway API defaulting on the
  generated `HTTPRoute`.
