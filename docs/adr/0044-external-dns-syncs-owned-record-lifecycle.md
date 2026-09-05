# ADR-0044: ExternalDNS synchronizes its owned DNS record lifecycle

- **Status:** Accepted
- **Date:** 2026-09-05
- **Related:** [ADR-0014](0014-argocd-app-of-apps-shared-helm-chart.md),
  [`k8s/externalDNS/README.md`](../../k8s/externalDNS/README.md)

## Context

ExternalDNS v0.22 requires an explicit lifecycle policy. The previous implicit
`upsert-only` behavior created and updated records but retained records after
their Kubernetes source was removed. That conflicts with this repository's
GitOps ownership model and leaves stale endpoints requiring manual cleanup.

The prd and sandbox instances have separate domain filters and TXT owner IDs.
The prd PowerDNS zone was verified to contain sixteen A records with sixteen
matching ownership TXT records. A sandbox canary verified that `sync` created
and then deleted only the canary A and ownership TXT records while preserving
the other six owned record pairs.

## Decision

Use `sync` in both active environments. Require each environment values file to
set the policy explicitly, and use the stable
`external-dns.kubernetes.io/hostname` annotation prefix.

ExternalDNS owns the complete lifecycle only for records selected by the
environment's domain filter and TXT owner ID. Records that must outlive their
Kubernetes source must not be assigned to that owner.

## Alternatives considered

- **`upsert-only`.** Prevents automated deletion but accumulates stale records
  after Git removes an application or hostname. *Rejected:* DNS would no longer
  converge to the declared state.
- **`create-only`.** Also prevents target updates, leaving records stale when a
  LoadBalancer or Gateway address changes. *Rejected.*

## Consequences

- Removing a managed HTTPRoute or annotated LoadBalancer Service also removes
  its A and ownership TXT records on the next reconciliation.
- The domain filter and TXT registry bound automated deletion; changing either
  setting requires reviewing the resulting ownership scope first.
- A rollback to `upsert-only` stops future deletions but does not restore an
  already deleted record. Restoring its Kubernetes source recreates it.
