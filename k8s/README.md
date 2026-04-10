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
| MeshCentral | - | `meshcentral.dev.butaco.net` | HTTPRoute |
| Loki | LoadBalancer (cluster-external log ingestion) | - | LoadBalancer |

### Secrets Management

- Encrypted with SOPS + Age
- Managed via ArgoCD helm-secrets CMP plugin
- Private key stored as Kubernetes secret `helm-secrets-private-keys` in `argocd` namespace

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
│   ├── values.yaml           # Schema: email, domain, cloudflare.apiToken
│   ├── secrets.enc.yaml      # Encrypted Cloudflare API token
│   ├── dev/values.yaml       # domain: dev.butaco.net
│   ├── prd/values.yaml       # domain: prd.butaco.net
│   └── templates/
│       ├── cluster-issuer.yaml    # letsencrypt-staging + letsencrypt-production
│       ├── certificate.yaml       # Wildcard cert: *.{domain}
│       ├── cloudflare-secret.yaml # Cloudflare API token secret
│       └── reference-grant.yaml  # Allows gateway-system to reference TLS secret
├── gateway/              # Cilium Gateway API (local Helm chart)
│   ├── Chart.yaml
│   ├── values.yaml           # Schema: domain
│   └── templates/
│       ├── gatewayclass.yaml # GatewayClass: cilium
│       └── gateway.yaml      # shared-gateway (HTTPS + HTTP listeners)
├── externalDNS/          # external-dns with PowerDNS
│   ├── chart/
│   │   ├── values.yaml
│   │   ├── secrets.enc.yaml
│   │   └── templates/
│   ├── values-common.yaml
│   ├── dev/values.yaml
│   └── prd/values.yaml
├── monitoring/           # Prometheus stack + Loki + exporters (prd only)
│   ├── apps/             # ArgoCD Application manifests
│   ├── charts/           # Local Helm charts (wrappers + HTTPRoutes)
│   └── values/           # Values per component
├── homepage/             # Homepage dashboard
│   └── chart/
└── meshcentral/          # MeshCentral remote management (dev only)
    └── chart/
```

## Initial Cluster Bootstrap

ArgoCD is deployed first via helmfile, then manages everything else via the App of Apps pattern.

```bash
# Register Age private key before deploying ArgoCD
kubectl create secret generic helm-secrets-private-keys \
  --from-file=key.txt=/path/to/age-private-key.txt \
  -n argocd

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

## Gateway

`shared-gateway` in `gateway-system` exposes:
- Port 443 (HTTPS) with wildcard TLS certificate
- Port 80 (HTTP) — available for non-TLS use if needed

Each service creates an HTTPRoute in its own namespace pointing to `shared-gateway`.
