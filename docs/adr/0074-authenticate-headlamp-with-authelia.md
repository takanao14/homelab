# ADR-0074: Authenticate Headlamp with Authelia

- **Status:** Accepted
- **Date:** 2026-10-15
- **Related:** [ADR-0067](0067-replace-authentik-with-lldap-and-authelia.md), [ADR-0015](0015-headlamp-per-cluster-in-cluster-deployment.md)

## Context

ADR-0067 allowed unauthenticated, read-only Headlamp access to avoid an identity
service dependency. Read-only access still exposes pod logs, ConfigMaps, and pod
specifications to every LAN client that can reach Headlamp.

## Decision

Replace ADR-0067's unauthenticated Headlamp access with Authelia forward auth
through Envoy Gateway, admitting only `lab-platform-admins`. Fail closed when
Authelia is unavailable. Restrict Headlamp ingress to the shared Gateway pods
with a NetworkPolicy so direct Service access cannot bypass authentication.

Keep the shared read-only ServiceAccount and defer API-server OIDC. Individual
Kubernetes authorization and audit identities are not required for this UI.
Writes and recovery inspection continue through the workstation admin kubeconfig.

## Consequences

Headlamp now depends on Authelia and LLDAP availability. Reuse the GPU Switch
forward-auth pattern; apply the Authelia access rule before syncing Headlamp.
Authentication does not expand Kubernetes permissions or provide per-user API
audit records. The ServiceAccount must retain its existing Secret and write
restrictions.
