# ArgoCD

ArgoCD configuration for Kubernetes cluster management via GitOps. Supports
`dev`, `prd`, and `sandbox`.

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
├── dev/
│   ├── helmfile.yaml         # Initial deployment config for dev
│   ├── values.yaml           # server.ingress.hostname: argocd.dev.butaco.net
│   ├── apps-values.yaml      # env: dev + enabled apps (see ADR-0014)
│   └── root-apps.yaml        # Bootstrap App of Apps for dev
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
| dev | dev-homelab | `argocd.dev.butaco.net` |
| prd | prd-homelab | `argocd.prd.butaco.net` |
| sandbox | sandbox-homelab | `http://argocd.sandbox.butaco.net` |

> `butaco.net` is a personal domain. Replace with your own domain in `dev/values.yaml` and `prd/values.yaml`.

## Initial Deployment

ArgoCD is initially deployed using helmfile, and subsequently self-manages itself.

```bash
# prd environment
cd k8s/argocd/prd
helmfile apply

# Apply root App of Apps
kubectl apply -f k8s/argocd/prd/root-apps.yaml
```

A helmfile hook will interrupt the deployment if the context of the target cluster is incorrect.

For sandbox, use `k8s/argocd/sandbox`. It intentionally exposes ArgoCD over
HTTP only and does not install cert-manager.

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

## Apps

| Application | Namespace | Environment |
|-------------|-----------|-------------|
| argocd | argocd | dev, prd, sandbox |
| cert-manager | cert-manager | dev, prd |
| cert-manager-config | cert-manager | dev, prd |
| comfyui | comfyui | dev only |
| external-secrets (eso) | external-secrets | dev, prd, sandbox |
| external-dns | external-dns | dev, prd, sandbox |
| gateway | gateway-system | dev, prd, sandbox |
| headlamp | headlamp | dev, prd |
| homepage | homepage | prd, sandbox |
| lemonade-server | lemonade-server | dev only |
| longhorn-ui | longhorn-system | sandbox only |
| meshcentral | meshcentral | dev only |
| monitoring | monitoring | dev, prd |
| ollama | ollama | dev only |
| open-webui | open-webui | dev only |
| reloader | reloader | dev, prd |

Sandbox intentionally uses HTTP only. Its Gateway has no HTTPS listener, and
cert-manager is not installed. ESO uses the `kubernetes-sandbox` OpenBao auth
mount, while external-dns manages `sandbox.butaco.net.` through PowerDNS.
Sandbox homepage is exposed at `http://homepage.sandbox.butaco.net` and reuses
the production dashboard Secret paths for staging validation. The sandbox
Longhorn UI is exposed through an authenticated reverse proxy instead of
routing directly to `longhorn-frontend`.
