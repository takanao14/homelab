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
├── dev/
│   ├── helmfile.yaml         # Initial deployment config for dev
│   ├── values.yaml           # server.ingress.hostname: argocd.dev.butaco.net
│   ├── root-apps.yaml        # Bootstrap App of Apps for dev
│   └── apps/                 # ArgoCD Application manifests
│       ├── argocd.yaml
│       ├── cert-manager-config.yaml
│       ├── cert-manager.yaml
│       ├── comfyui.yaml
│       ├── eso.yaml
│       ├── external-dns.yaml
│       ├── gateway.yaml
│       ├── lemonade-server.yaml
│       ├── meshcentral.yaml
│       ├── monitoring.yaml       # Prometheus agent mode (k8s/dev-monitoring)
│       ├── ollama.yaml
│       ├── open-webui.yaml
│       └── reloader.yaml
├── prd/
│   ├── helmfile.yaml         # Initial deployment config for prd
│   ├── values.yaml           # server.ingress.hostname: argocd.prd.butaco.net
│   ├── root-apps.yaml        # Bootstrap App of Apps for prd
│   └── apps/                 # ArgoCD Application manifests
│       ├── argocd.yaml
│       ├── cert-manager-config.yaml
│       ├── cert-manager.yaml
│       ├── eso.yaml
│       ├── external-dns.yaml
│       ├── gateway.yaml
│       ├── homepage.yaml
│       ├── monitoring.yaml       # Full monitoring stack (k8s/monitoring)
│       └── reloader.yaml
└── sandbox/
    ├── helmfile.yaml         # Initial deployment config for sandbox
    ├── values.yaml           # HTTP route: argocd.sandbox.butaco.net
    ├── root-apps.yaml        # Bootstrap App of Apps for sandbox
    └── apps/
        ├── argocd.yaml
        ├── eso.yaml
        ├── external-dns.yaml
        ├── gateway.yaml
        └── longhorn-ui.yaml
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
| homepage | homepage | prd only |
| lemonade-server | lemonade-server | dev only |
| longhorn-ui | longhorn-system | sandbox only |
| meshcentral | meshcentral | dev only |
| monitoring | monitoring | dev, prd |
| ollama | ollama | dev only |
| open-webui | open-webui | dev only |
| reloader | reloader | dev, prd |

Sandbox intentionally uses HTTP only. Its Gateway has no HTTPS listener, and
cert-manager is not installed. ESO uses the `kubernetes-sandbox` OpenBao auth
mount, while external-dns manages `sandbox.butaco.net.` through PowerDNS. The
sandbox Longhorn UI is exposed through an authenticated reverse proxy instead
of routing directly to `longhorn-frontend`.
