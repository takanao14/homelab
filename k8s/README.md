# k8s

Kubernetes manifests and Helm charts for homelab clusters managed via ArgoCD GitOps.

## Environments

| Environment | Domain | Cluster |
|-------------|--------|---------|
| prd | `*.prd.butaco.net` | prd-homelab |
| dev | `*.dev.butaco.net` | dev-homelab |
| sandbox | `*.sandbox.butaco.net` (HTTP only) | sandbox-homelab |

> **Note**: `butaco.net` is a personal domain. Replace it with your own domain before use.
> Search for `butaco.net` across `k8s/` and update all occurrences in values files.

## Architecture

### Networking

- **CNI**: Cilium 1.19.x
- **Ingress**: Cilium Gateway API (Gateway API v1.4.1 experimental)
- **TLS**: cert-manager wildcard certificate via Cloudflare DNS-01 challenge
  for prd/dev; sandbox intentionally uses HTTP without cert-manager
- **DNS**: external-dns with PowerDNS provider (`gateway-httproute` source)

All HTTP services are exposed via HTTPRoute referencing a shared Gateway (`shared-gateway` in `gateway-system` namespace). TLS is terminated at the Gateway using a wildcard certificate.

### Secrets Management

- All Kubernetes secrets are managed via [External Secrets Operator](https://external-secrets.io/) (ESO)
- ESO fetches secrets from OpenBao KV v2 (Vault-compatible) using Kubernetes auth
- `ClusterSecretStore` named `openbao` is configured by the `eso` chart
- OpenBao is deployed and managed via Ansible (`ansible/roles/openbao`)

## Directory Structure

```
k8s/
├── argocd/               # ArgoCD self-management + App of Apps
│   ├── values-common.yaml
│   ├── chart/                # Helm chart for ArgoCD HTTPRoute (shared by prd/dev)
│   │   └── templates/
│   │       └── httproute.yaml    # Uses server.ingress.hostname from values
│   ├── dev/
│   │   ├── helmfile.yaml
│   │   ├── values.yaml           # server.ingress.hostname: argocd.dev.butaco.net
│   │   ├── root-apps.yaml        # Bootstrap App of Apps for dev
│   │   └── apps/                 # ArgoCD Application manifests
│   └── prd/
│       ├── helmfile.yaml
│       ├── values.yaml           # server.ingress.hostname: argocd.prd.butaco.net
│       ├── root-apps.yaml        # Bootstrap App of Apps for prd
│       └── apps/                 # ArgoCD Application manifests
├── cert-manager/         # Wildcard certificate config (local Helm chart)
│   ├── Chart.yaml
│   ├── values.yaml           # Schema: email, domain
│   ├── dev/values.yaml       # domain: dev.butaco.net
│   ├── prd/values.yaml       # domain: prd.butaco.net
│   └── templates/
│       ├── cluster-issuer.yaml          # letsencrypt-staging + letsencrypt-production
│       ├── certificate.yaml             # Wildcard cert: *.{domain}
│       ├── cloudflare-external-secret.yaml  # ESO ExternalSecret for Cloudflare API token
│       └── reference-grant.yaml         # Allows gateway-system to reference TLS secret
├── eso/                  # External Secrets Operator + ClusterSecretStore (OpenBao)
│   ├── Chart.yaml
│   ├── values.yaml
│   └── templates/
│       ├── cluster-secret-store.yaml  # ClusterSecretStore pointing to OpenBao
│       └── token-reviewer.yaml        # ServiceAccount for OpenBao Kubernetes auth
├── gateway/              # Cilium Gateway API (local Helm chart)
│   ├── Chart.yaml
│   ├── values.yaml           # Schema: domain
│   └── templates/
│       └── gateway.yaml      # shared-gateway (configurable HTTP/HTTPS listeners)
├── externalDNS/          # external-dns with PowerDNS
│   ├── chart/
│   │   ├── values.yaml
│   │   └── templates/
│   │       ├── deployment.yaml
│   │       ├── rbac.yaml
│   │       └── external-secret.yaml  # ESO ExternalSecret for PowerDNS API key
│   ├── values-common.yaml
│   ├── dev/values.yaml
│   └── prd/values.yaml
├── monitoring/           # Prometheus stack + Loki + exporters (prd only)
│   ├── apps/             # ArgoCD Application manifests
│   ├── charts/           # Local Helm charts (wrappers + HTTPRoutes)
│   └── values/           # Values per component
├── dev-monitoring/       # Prometheus agent mode (dev cluster → remote_write to prd)
│   ├── charts/prometheus/    # kube-prometheus-stack wrapper (agent mode)
│   └── values/prometheus.yaml
├── reloader/             # Stakater Reloader (auto-restart on Secret/ConfigMap change)
│   ├── Chart.yaml
│   └── values.yaml
├── comfyui/              # ComfyUI AI image generation (dev only, AMD GPU)
│   ├── values.yaml
│   └── chart/
├── lemonade-server/      # Lemonade LLM inference server (dev only, AMD GPU)
│   ├── values.yaml
│   └── chart/
├── ollama/               # Ollama LLM server (dev only, AMD GPU)
│   ├── values.yaml
│   └── chart/
├── homepage/             # Homepage dashboard (prd only)
│   └── chart/
└── meshcentral/          # MeshCentral remote management (dev only)
    └── chart/
```

## Initial Cluster Bootstrap

ArgoCD is deployed first via helmfile, then manages everything else via the App of Apps pattern.

```bash
# Deploy ArgoCD (prd)
cd k8s/argocd/prd
helmfile apply

# Apply root App of Apps
kubectl apply -f k8s/argocd/prd/root-apps.yaml
```

After `root-apps.yaml` is applied, ArgoCD syncs all applications automatically.
