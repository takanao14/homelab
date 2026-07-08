# ADR-0015: Headlamp runs in-cluster per cluster instead of a central multi-cluster UI

- **Status:** Accepted
- **Date:** 2026-07-08
- **Related:** [`k8s/headlamp/README.md`](../../k8s/headlamp/README.md), [ADR-0012](0012-openbao-eso-cluster-rebuild-registration.md), [ADR-0014](0014-argocd-app-of-apps-shared-helm-chart.md)

## Context

Headlamp originally ran only in the prd cluster with `-in-cluster=false`,
mounting static kubeconfigs for both prd and dev (stored in OpenBao at
`kubeconfig/*`, synced by ESO) to present both clusters in a single UI. That
gave one URL and one login, but:

- the mounted kubeconfigs were long-lived cluster-admin credentials that
  crossed cluster boundaries: compromising the prd Headlamp (or its Gateway
  path) reached dev as well;
- every k0s cluster rebuild changed the API server CA and credentials, so the
  stored kubeconfigs went stale and needed a manual refresh — the same class
  of post-rebuild friction as the OpenBao/ESO re-registration in ADR-0012;
- the UI for every cluster depended on prd being up;
- each additional cluster (e.g. sandbox) required another kubeconfig secret
  and OpenBao policy grant.

Meanwhile ADR-0014 reduced per-environment app enablement to a two-line
change, removing most of the operational cost that originally favored a
single deployment.

## Decision

Deploy Headlamp in-cluster in each environment that needs it (prd and dev;
sandbox stays minimal per ADR-0010). The upstream chart default
(`config.inCluster: true`) is used: authentication is ServiceAccount RBAC
plus a per-cluster login token Secret, and no kubeconfig material exists
anywhere.

- Common values live in the wrapper chart (`k8s/headlamp/chart/values.yaml`);
  per-env values (`k8s/headlamp/<env>/values.yaml`) carry only the hostname.
- The ESO `ExternalSecret` for kubeconfigs is removed. The OpenBao
  `kubeconfig/*` entries remain — they serve workstation kubeconfig sync
  (`scripts/secrets/get-kubeconfig.sh`), not Headlamp.

The rejected alternative — keeping the central UI with read-only scoped
kubeconfigs — would have shrunk the blast radius but kept the rebuild
staleness and the availability coupling.

## Consequences

- A rebuilt cluster gets a working Headlamp from the app-of-apps bootstrap
  with zero manual secret steps; only the login token Secret
  (`headlamp-token`, created manually, not ArgoCD-managed) must be recreated.
- No single-pane multi-cluster view: each cluster has its own URL
  (`headlamp.<env>.butaco.net`) and its own login token. Homepage links can
  aggregate the URLs.
- If a cluster's Gateway path is broken, that cluster's Headlamp is
  unreachable from outside; debugging then falls back to kubectl/k9s. The
  central model could inspect a remote cluster's API in that situation, and
  this trade-off was accepted.
- Each instance binds its ServiceAccount to `cluster-admin` (upstream chart
  default), scoped to its own cluster only.
