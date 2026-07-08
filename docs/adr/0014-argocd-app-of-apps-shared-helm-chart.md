# ADR-0014: ArgoCD App of Apps rendered from a shared Helm chart

- **Status:** Accepted
- **Date:** 2026-07-08
- **Related:** [`k8s/argocd/README.md`](../../k8s/argocd/README.md), [ADR-0010](0010-sandbox-argocd-uses-http-only-gitops-bootstrap.md)

## Context

Each environment (prd / dev / sandbox) ran its own App of Apps rooted at a
per-environment directory of plain Application manifests
(`k8s/argocd/<env>/apps/`). Diffing the shared applications across
environments showed that the manifests were near-total duplicates: the only
real differences were the environment name inside values paths and a handful
of inline `values:` blocks. Environment differences were expressed in three
inconsistent styles (relative `valueFiles`, multi-source `$values` refs, and
inline `values:` blocks embedded in Application manifests).

Deploying an existing application to another environment therefore meant
copying a manifest and hand-editing environment strings, which had already
produced small drifts (sync-wave and comment mismatches) and made the
prd/sandbox parity requested for staging validation expensive to maintain.

An ApplicationSet-based alternative was considered and rejected: the
application shapes are heterogeneous (upstream charts with `$values`
multi-source, local charts, and app-of-apps directories), which fits Helm
per-app templates better than a single ApplicationSet template, and it would
add a controller-level abstraction for a fleet of only three in-cluster
ArgoCD instances.

## Decision

1. Normalize environment differences into per-app values files first:
   inline `values:` blocks in Application manifests were moved to
   `k8s/<app>/<env>/values.yaml` (local charts use relative `valueFiles`;
   upstream charts use multi-source `$values` refs, e.g.
   `k8s/cert-manager/controller/<env>/values.yaml` and
   `k8s/open-webui/<env>/values.yaml`).
2. Replace the per-environment `apps/` manifest directories with a single
   shared Helm chart, `k8s/argocd/apps/`, containing one Application template
   per app. Templates are gated by `apps.<name>.enabled` and substitute
   `{{ .Values.env }}` into values paths.
3. Each environment declares its app set in
   `k8s/argocd/<env>/apps-values.yaml` (`env`, enabled flags, per-env
   overrides such as the monitoring source, which is the only structurally
   divergent app: prd runs the central kube-prometheus-stack app-of-apps,
   dev runs the Prometheus agent chart, sandbox runs a standalone
   kube-prometheus-stack).
4. `root-apps.yaml` points at the chart with
   `helm.valueFiles: [../<env>/apps-values.yaml]`.
5. Upstream chart coordinates (`repoURL` / `chart` / `targetRevision`) live in
   the chart's `values.yaml`, not in `templates/`: Renovate ignores
   `**/templates/**`, and the existing regex manager tracks exactly that
   key triple in `k8s/**/*.yaml`.

The migration was validated by rendering the chart per environment and
comparing key-sorted JSON against the previous manifests: all 35 Applications
across the three environments were semantically identical, so the switchover
was a no-op for the clusters. Sandbox was switched first, then dev and prd.

## Consequences

- Deploying an app to another environment is now two small changes:
  set `apps.<name>.enabled: true` in `k8s/argocd/<env>/apps-values.yaml` and
  add `k8s/<app>/<env>/values.yaml` if the app takes per-env values.
- Application manifests are no longer directly readable in git; use
  `helm template k8s/argocd/apps -f k8s/argocd/<env>/apps-values.yaml`
  (or the ArgoCD UI) to inspect the rendered Applications.
- Adding a new application requires one template in
  `k8s/argocd/apps/templates/` plus a defaults entry in the chart's
  `values.yaml`, instead of one manifest per environment.
- Renovate keeps tracking upstream chart versions from the chart's
  `values.yaml`; new upstream-chart apps must keep the
  `repoURL:`/`chart:`/`targetRevision:` key order there for the regex
  manager to match.
- The bootstrap flow is unchanged: helmfile installs ArgoCD, then
  `kubectl apply -f k8s/argocd/<env>/root-apps.yaml` starts the App of Apps.
