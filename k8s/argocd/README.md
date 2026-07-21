# ArgoCD

ArgoCD configuration for Kubernetes cluster management via GitOps. Supports
`prd` and `sandbox`.

## Directory Structure

```
argocd/
├── values-common.yaml        # Common Helm values (CMP plugin, insecure mode)
├── chart/                    # Shared Helm chart for ArgoCD HTTPRoute
│   └── templates/
│       └── httproute.yaml    # Uses server.ingress.hostname from values
├── apps/                     # App-of-apps chart: one Application template per app
│   ├── Chart.yaml
│   ├── values.yaml           # All apps disabled by default; waves; upstream chart versions
│   └── templates/            # Gated by apps.<name>.enabled, env via {{ .Values.env }}
├── prd/
│   ├── helmfile.yaml         # Initial deployment config for prd
│   ├── values.yaml           # server.ingress.hostname: argocd.prd.butaco.net
│   ├── apps-values.yaml      # env: prd + enabled apps
│   └── root-apps.yaml        # Bootstrap App of Apps for prd
└── sandbox/
    ├── helmfile.yaml         # Initial deployment config for sandbox
    ├── values.yaml           # HTTP route: argocd.sandbox.butaco.net
    ├── apps-values.yaml      # env: sandbox + enabled apps
    └── root-apps.yaml        # Bootstrap App of Apps for sandbox
```

## Environments

| Environment | Cluster | ArgoCD URL |
|-------------|---------|------------|
| prd | prd-homelab | `argocd.prd.butaco.net` |
| sandbox | sandbox-homelab | `http://argocd.sandbox.butaco.net` |

> `butaco.net` is a personal domain. Replace with your own domain in `prd/values.yaml` and `sandbox/values.yaml`.

## Initial Deployment

ArgoCD is initially deployed using helmfile, and subsequently self-manages itself.

> **helmfile is for bootstrap only — never run `helmfile apply` against a
> cluster that already runs ArgoCD.** Once `root-apps.yaml` is applied, the
> `argocd` Application takes over this release and re-running helmfile fails
> (see [Changing values](#changing-values) below).

```bash
# prd environment
cd k8s/argocd/prd
helmfile apply

# Apply root App of Apps
kubectl apply -f k8s/argocd/prd/root-apps.yaml
```

Two helmfile hooks guard this step: one interrupts the deployment if the target
cluster context is wrong, the other if ArgoCD already self-manages the release.
The second can be overridden with `ARGOCD_BOOTSTRAP_FORCE=1`, which should only
be needed when deliberately re-bootstrapping.

For sandbox, use `k8s/argocd/sandbox`. It intentionally exposes ArgoCD over
HTTP only and does not install cert-manager.

### Changing values

After bootstrap, `values-common.yaml` and `<env>/values.yaml` are read by
ArgoCD straight from git. **Commit and push — that is the whole apply step.**
The `argocd` Application has `selfHeal: true` and syncs on its own.

Running `helmfile apply` instead fails with:

```
invalid ownership metadata; annotation validation error:
missing key "meta.helm.sh/release-name"
```

ArgoCD applies with `ServerSideApply=true` and does not write Helm's ownership
annotations, so every resource introduced by a chart version newer than the
bootstrap one exists without them. Helm refuses to adopt those resources into
its release. The Helm release record therefore stops at the bootstrap version
while the live cluster tracks the chart version in `apps/values.yaml`; this
divergence is expected, not a fault to repair. A fresh-cluster bootstrap is
unaffected because no conflicting resources exist yet.

## Secrets Management

All application secrets are managed via External Secrets Operator (ESO) backed by OpenBao — see `k8s/eso/` for the `ClusterSecretStore` configuration.

## HTTPRoute

ArgoCD is exposed via Gateway API HTTPRoute. The hostname is configured in each environment's `values.yaml` (`server.ingress.hostname`) and rendered by the shared `chart/` Helm chart.

The `argocd.yaml` Application uses multi-source:
1. Upstream `argo-cd` Helm chart
2. Values ref (this repo)
3. `k8s/argocd/chart` — renders HTTPRoute from values

## App of Apps

Each environment's `root-apps.yaml` points at the shared `apps/` chart with
`helm.valueFiles: [../<env>/apps-values.yaml]`. The chart renders one
Application per enabled app; per-app environment differences live in
`k8s/<app>/<env>/values.yaml` files referenced by the generated Applications
(see [ADR-0014](../../docs/adr/0014-argocd-app-of-apps-shared-helm-chart.md)).

To deploy an app to another environment:

1. Set `apps.<name>.enabled: true` in `k8s/argocd/<env>/apps-values.yaml`.
2. Add `k8s/<app>/<env>/values.yaml` if the app takes per-env values.

To add a new application, add a template to `apps/templates/` and a defaults
entry (enabled: false, wave) to `apps/values.yaml`. Keep upstream chart
coordinates in `apps/values.yaml` — Renovate ignores `**/templates/**`, and
its regex manager matches the `repoURL:` / `chart:` / `targetRevision:` key
order there.

Inspect the rendered Applications with:

```bash
helm template k8s/argocd/apps -f k8s/argocd/<env>/apps-values.yaml
```

### Sync waves

Waves are defined once in `apps/values.yaml` and shared by all environments:

| Wave | Applications | Rationale |
|------|--------------|-----------|
| -2 | envoy-gateway-crds | Gateway API CRDs (sole owner) |
| -1 | envoy-gateway | Controller after its CRDs |
| 0 | cert-manager, eso, gateway | Foundation: CRDs, ClusterSecretStore, shared Gateway |
| 1 | everything else | Consumers of wave 0 (ExternalSecrets, HTTPRoutes, issuers) |
| 2 | longhorn-ui | Behind the authenticated Gateway route (ADR-0009) |

Wave gating relies on the Application health check re-enabled in
`values-common.yaml`; automated syncs never retry the same revision, so
`root-apps` and the CRD-racing apps carry explicit retry policies.

Known caveat: the Gateway HTTPS listener (wave 0) references the wildcard TLS
Secret and ReferenceGrant created by cert-manager-config (wave 1). On a fresh
prd cluster the listener reports `ResolvedRefs=False` until wave 1 syncs; if
the gateway app sticks at Progressing/Degraded in wave 0, trigger a manual sync
of cert-manager-config to create the Secret, then re-sync root-apps.

## Apps

| Application | Namespace | Environment |
|-------------|-----------|-------------|
| argocd | argocd | prd, sandbox |
| cert-manager | cert-manager | prd |
| cert-manager-config | cert-manager | prd |
| comfyui | comfyui | prd |
| external-secrets (eso) | external-secrets | prd, sandbox |
| external-dns | external-dns | prd, sandbox |
| gateway | gateway-system | prd, sandbox |
| headlamp | headlamp | prd |
| homepage | homepage | prd, sandbox |
| lemonade-server | lemonade-server | prd |
| longhorn-ui | longhorn-system | sandbox only |
| monitoring | monitoring (argocd in prd) | prd, sandbox |
| ollama | ollama | prd |
| open-webui | open-webui | prd |
| reloader | reloader | prd |

Sandbox intentionally uses HTTP only. Its Gateway has no HTTPS listener, and
cert-manager is not installed. ESO uses the `kubernetes-sandbox` OpenBao auth
mount, while external-dns manages `sandbox.butaco.net.` through PowerDNS.
Sandbox homepage is exposed at `http://homepage.sandbox.butaco.net` and reuses
the production dashboard Secret paths for staging validation. The sandbox
Longhorn UI is exposed through an authenticated reverse proxy instead of
routing directly to `longhorn-frontend`.
