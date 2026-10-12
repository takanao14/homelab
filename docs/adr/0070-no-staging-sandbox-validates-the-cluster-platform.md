# ADR-0070: No staging environment; sandbox validates the cluster platform

- **Status:** Accepted
- **Date:** 2026-10-12
- **Related:** [ADR-0010](0010-sandbox-argocd-uses-http-only-gitops-bootstrap.md), [ADR-0012](0012-openbao-eso-cluster-rebuild-registration.md), [ADR-0019](0019-merge-gpu-worker-into-prd-retire-dev-cluster.md), [ADR-0033](0033-sandbox-storage-openebs-localpv-and-truenas-nfs.md), [ADR-0067](0067-replace-authentik-with-lldap-and-authelia.md)

## Context

Retiring dev (ADR-0019) left prd as the only long-lived cluster and sandbox as
the only other one, without deciding whether sandbox should take over dev's
role. It drifted toward a partial staging copy: it runs prd applications such
as homepage, pdns-ui, and Headlamp, yet differs from prd in exactly the areas
that break prd changes. It is HTTP-only (ADR-0010), outside single sign-on
(ADR-0067), and has no GPU worker. The forward-auth and GPU changes of the LDAP
migration could not be tried there and were validated against prd with
rendering, server-side diffs and dry runs, and local reproduction instead.

Running those applications also required prd credentials. The sandbox OpenBao
auth mount has the same `k8s-external-dns`, `k8s-homepage`, and
`k8s-monitoring` policies as prd, so a disposable, unauthenticated HTTP cluster
can read the PowerDNS API key, which can rewrite every zone, and the Proxmox
API tokens. ADR-0010 meant sandbox policy to stay narrower than prd.

## Decision

There is no staging environment. A prd change is validated before merge with
`helm template` using the Argo CD value files, `kubectl diff --server-side` or a
server-side dry run, `ansible-playbook --check`, and local reproduction when
behavior matters. A bad change is reverted in Git.

Sandbox is a disposable cluster for validating the cluster platform:

- **In scope:** cluster creation and rebuild with k0sctl, k0s, Cilium, Envoy
  Gateway, Argo CD and App of Apps bootstrap, ESO with OpenBao, storage
  (ADR-0033), the monitoring stack, and node drain, shutdown, and startup
  procedures.
- **Out of scope:** TLS, single sign-on, GPU workloads, and applications that
  need prd credentials.
- **Workloads:** only the platform set and workloads that exercise a platform
  path end to end. A prd application is not deployed to sandbox to mirror prd.
- **Credentials:** sandbox never reads prd credentials. Its OpenBao auth mount
  is granted only paths dedicated to sandbox, and a dedicated test secret
  exercises the OpenBao to ESO path.
- **DNS:** external-dns is not deployed. A wildcard record for
  `*.sandbox.butaco.net` points to the fixed sandbox Gateway address, so
  sandbox holds no DNS credentials.

## Alternatives considered

- **Make sandbox a staging copy of prd.** It would need cert-manager or external
  TLS termination, Authelia, prd-equivalent applications, and prd credentials
  on a cluster that is rebuilt often. *Rejected.*
- **Add a separate staging cluster.** This restores the resource and upkeep cost
  that retiring dev removed. *Rejected.*
- **Keep external-dns in sandbox with a TSIG key limited to the sandbox zone.**
  It keeps external-dns upgrades testable, but adds RFC2136 configuration on
  the PowerDNS primary for one zone. *Rejected* in favor of a wildcard record.

## Consequences

- homepage and pdns-ui leave sandbox, and Headlamp stays only as a read-only
  inspection tool that needs no secrets. Monitoring stays only in a form that
  reads no prd secrets.
- The sandbox OpenBao auth mount loses `k8s-external-dns`, `k8s-homepage`, and
  `k8s-monitoring`.
- external-dns upgrades are validated against prd with diffs only.
- Changes involving TLS, single sign-on, or GPU workloads have no rehearsal
  environment and rely on pre-merge checks and Git revert.
