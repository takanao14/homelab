# k8s

Kubernetes manifests and Helm charts for homelab clusters managed via ArgoCD GitOps.

## Environments

| Environment | Domain | Cluster |
|-------------|--------|---------|
| prd | `*.prd.butaco.net` | prd-homelab |
| dev | `*.dev.butaco.net` | dev-homelab |

> **Note**: `butaco.net` is a personal domain. Replace it with your own domain before use.
> Search for `butaco.net` across `k8s/` and update all occurrences in values files.

## Architecture

### Networking

- **CNI**: Cilium 1.19.x
- **Ingress**: Cilium Gateway API (Gateway API v1.4.1 experimental)
- **TLS**: cert-manager wildcard certificate via Cloudflare DNS-01 challenge
- **DNS**: external-dns with PowerDNS provider (`gateway-httproute` source)

All HTTP services are exposed via HTTPRoute referencing a shared Gateway (`shared-gateway` in `gateway-system` namespace). TLS is terminated at the Gateway using a wildcard certificate.

### Service Access

| Service | URL (prd) | URL (dev) | Method |
|---------|-----------|-----------|--------|
| ArgoCD | `argocd.prd.butaco.net` | `argocd.dev.butaco.net` | HTTPRoute |
| Homepage | `www.prd.butaco.net` | - | HTTPRoute |
| Grafana | `grafana.prd.butaco.net` | - | HTTPRoute |
| Prometheus | `prometheus.prd.butaco.net` | - | HTTPRoute |
| Loki | LoadBalancer (cluster-external log ingestion) | - | LoadBalancer |
| MeshCentral | - | `meshcentral.dev.butaco.net` | HTTPRoute |
| ComfyUI | - | `comfyui.dev.butaco.net` | HTTPRoute |
| Ollama | - | `ollama.dev.butaco.net` | HTTPRoute |
| Open-WebUI | - | `open-webui.dev.butaco.net` | HTTPRoute |

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
│       ├── gatewayclass.yaml # GatewayClass: cilium
│       └── gateway.yaml      # shared-gateway (HTTPS + HTTP listeners)
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

## cert-manager

Wildcard certificate issued via Let's Encrypt production using Cloudflare DNS-01 challenge.

- Certificate: `*.prd.butaco.net` → Secret `wildcard-prd-butaco-net-tls` in `cert-manager` namespace
- ReferenceGrant allows `gateway-system` to reference the TLS secret
- `--dns01-recursive-nameservers=8.8.8.8:53,1.1.1.1:53` is required to bypass internal DNS (PowerDNS) for ACME validation
- Cloudflare API token is fetched from OpenBao via ESO (`k8s/cert-manager/cloudflare` → `api-token`)

## Gateway

`shared-gateway` in `gateway-system` exposes:
- Port 443 (HTTPS) with wildcard TLS certificate
- Port 80 (HTTP) — available for non-TLS use if needed

Each service creates an HTTPRoute in its own namespace pointing to `shared-gateway`.
