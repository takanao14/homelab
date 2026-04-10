# ArgoCD

ArgoCD configuration for Kubernetes cluster management via GitOps. Supports two environments: `dev` and `prd`.

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
│       ├── cert-manager.yaml
│       ├── cert-manager-config.yaml
│       ├── gateway.yaml
│       ├── external-dns.yaml
│       └── meshcentral.yaml
└── prd/
    ├── helmfile.yaml         # Initial deployment config for prd
    ├── values.yaml           # server.ingress.hostname: argocd.prd.butaco.net
    ├── root-apps.yaml        # Bootstrap App of Apps for prd
    └── apps/                 # ArgoCD Application manifests
        ├── argocd.yaml
        ├── cert-manager.yaml
        ├── cert-manager-config.yaml
        ├── gateway.yaml
        ├── external-dns.yaml
        ├── homepage.yaml
        └── monitoring.yaml
```

## Environments

| Environment | Cluster | ArgoCD URL |
|-------------|---------|------------|
| dev | dev-homelab | `argocd.dev.butaco.net` |
| prd | prd-homelab | `argocd.prd.butaco.net` |

> `butaco.net` is a personal domain. Replace with your own domain in `dev/values.yaml` and `prd/values.yaml`.

## Initial Deployment

ArgoCD is initially deployed using helmfile, and subsequently self-manages itself.

```bash
# Register Age private key before deploying ArgoCD
kubectl create secret generic helm-secrets-private-keys \
  --from-file=key.txt=/path/to/age-private-key.txt \
  -n argocd

# prd environment
cd k8s/argocd/prd
helmfile apply

# Apply root App of Apps
kubectl apply -f k8s/argocd/prd/root-apps.yaml
```

A helmfile hook will interrupt the deployment if the context of the target cluster is incorrect.

## helm-secrets Plugin (CMP)

A Config Management Plugin (CMP) defined in `values-common.yaml` allows SOPS-encrypted secrets to be managed in Git.

- Encryption method: Age
- Custom image: `ghcr.io/takanao14/argocd-helm-secrets-cmp:latest`
- Private key is managed via a Kubernetes secret (`helm-secrets-private-keys`)

## HTTPRoute

ArgoCD is exposed via Gateway API HTTPRoute. The hostname is configured in each environment's `values.yaml` (`server.ingress.hostname`) and rendered by the shared `chart/` Helm chart.

The `argocd.yaml` Application uses multi-source:
1. Upstream `argo-cd` Helm chart
2. Values ref (this repo)
3. `k8s/argocd/chart` — renders HTTPRoute from values

## Apps

| Application | Namespace | Environment | Secrets |
|-------------|-----------|-------------|---------|
| argocd | argocd | dev, prd | none |
| cert-manager | cert-manager | dev, prd | none |
| cert-manager-config | cert-manager | dev, prd | yes (helm-secrets) |
| gateway | gateway-system | dev, prd | none |
| external-dns | dns-homelab | dev, prd | yes (helm-secrets) |
| homepage | homepage | prd only | yes (helm-secrets) |
| monitoring | monitoring | prd only | yes (helm-secrets) |
| meshcentral | meshcentral | dev only | none |
