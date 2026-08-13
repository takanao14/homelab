# k8s

Kubernetes manifests and Helm charts for homelab clusters managed via ArgoCD GitOps.

## Environments

| Environment | Domain | Cluster |
|-------------|--------|---------|
| prd | `*.prd.butaco.net` | prd-homelab |
| sandbox | `*.sandbox.butaco.net` (HTTP only) | sandbox-homelab |

> **Note**: `butaco.net` is a personal domain. Replace it with your own domain before use.
> Search for `butaco.net` across `k8s/` and update all occurrences in values files.

## Architecture

### Networking

- **CNI**: Cilium 1.19.x
- **Ingress**: Envoy Gateway via Gateway API (Gateway API v1.5.1 experimental)
- **TLS**: cert-manager wildcard certificate via Cloudflare DNS-01 challenge
  for prd; sandbox intentionally uses HTTP without cert-manager
- **DNS**: external-dns with PowerDNS provider (`gateway-httproute` source)

HTTPRoutes use `gateway-system/shared-gateway-envoy`. The Gateway terminates prd
TLS with a wildcard certificate; sandbox remains HTTP-only. See
[`ADR-0011`](../docs/adr/0011-cilium-gateway-to-envoy-gateway-migration.md) for
the Cilium Gateway to Envoy Gateway migration decision.

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
│   ├── chart/                # Helm chart for ArgoCD HTTPRoute
│   │   └── templates/
│   │       └── httproute.yaml    # Uses server.ingress.hostname from values
│   ├── apps/                 # App-of-apps chart (one Application template per app, ADR-0014)
│   │   ├── Chart.yaml
│   │   ├── values.yaml           # Defaults: apps disabled, waves, upstream chart versions
│   │   └── templates/
│   ├── prd/
│   │   ├── helmfile.yaml
│   │   ├── values.yaml           # server.ingress.hostname: argocd.prd.butaco.net
│   │   ├── apps-values.yaml      # env: prd + enabled apps
│   │   └── root-apps.yaml        # Bootstrap App of Apps for prd
│   └── sandbox/
│       ├── helmfile.yaml
│       ├── values.yaml           # server.ingress.hostname: argocd.sandbox.butaco.net
│       ├── apps-values.yaml      # env: sandbox + enabled apps
│       └── root-apps.yaml        # Bootstrap App of Apps for sandbox
├── cert-manager/         # Wildcard certificate config (local Helm chart)
│   ├── Chart.yaml
│   ├── values.yaml           # Schema: email, domain
│   ├── prd/values.yaml       # domain: prd.butaco.net
│   ├── controller/           # Values for the upstream cert-manager chart (common + per-env)
│   └── templates/
│       ├── cluster-issuer.yaml          # letsencrypt-staging + letsencrypt-production
│       ├── certificate.yaml             # Wildcard cert: *.{domain}
│       ├── cloudflare-external-secret.yaml  # ESO ExternalSecret for Cloudflare API token
│       └── reference-grant.yaml         # Allows gateway-system to reference TLS secret
├── envoy-gateway/        # Envoy Gateway controller + Gateway API CRDs (sole CRD owner, ADR-0011)
│   ├── crds/                 # CRD-only wrapper chart (sync wave -2)
│   └── controller/           # gateway-helm wrapper values (sync wave -1)
├── eso/                  # External Secrets Operator + ClusterSecretStore (OpenBao)
│   ├── Chart.yaml
│   ├── values.yaml
│   ├── {prd,sandbox}/values.yaml  # openbao.mountPath per environment
│   └── templates/
│       ├── cluster-secret-store.yaml  # ClusterSecretStore pointing to OpenBao
│       └── auth-delegator.yaml        # TokenReview RBAC for the ESO ServiceAccount
├── gateway/              # Shared Envoy Gateway API resources (local Helm chart)
│   ├── Chart.yaml
│   ├── values.yaml           # Schema: domain
│   ├── {prd,sandbox}/values.yaml  # domain per environment; sandbox disables HTTPS
│   └── templates/
│       ├── envoyproxies.yaml
│       ├── gatewayclasses.yaml
│       └── gateways.yaml     # shared-gateway-envoy (configurable HTTP/HTTPS listeners)
├── externalDNS/          # external-dns with PowerDNS
│   ├── chart/
│   │   ├── values.yaml
│   │   └── templates/
│   │       ├── deployment.yaml
│   │       ├── rbac.yaml
│   │       └── external-secret.yaml  # ESO ExternalSecret for PowerDNS API key
│   ├── values-common.yaml
│   ├── prd/values.yaml
│   └── sandbox/values.yaml
├── monitoring/           # Prometheus stack + Loki + exporters (prd full stack; sandbox subset)
│   ├── apps/             # Helm chart rendering the monitoring ArgoCD Applications
│   ├── charts/           # Local Helm charts (wrappers + HTTPRoutes + dashboards)
│   ├── dashboards/       # Dashboard generator (Go, grafana-foundation-sdk)
│   └── values/           # Values per component (+ apps-sandbox.yaml subset overlay)
├── reloader/             # Stakater Reloader (auto-restart on Secret/ConfigMap change)
│   ├── Chart.yaml
│   └── values.yaml
├── comfyui/              # ComfyUI AI image generation (prd, AMD GPU)
│   ├── values.yaml
│   └── chart/
├── lemonade-server/      # Lemonade LLM inference server (prd, AMD GPU)
│   ├── values.yaml
│   └── chart/
├── ollama/               # Ollama LLM server (prd, AMD GPU)
│   ├── values.yaml
│   └── chart/
├── vllm/                 # vLLM OpenAI-compatible server (prd, AMD GPU)
│   ├── values.yaml
│   └── chart/
├── gpu-switch/           # Authenticated UI for exclusive GPU workload switching (prd)
│   ├── app/                  # Dependency-free Go backend and embedded UI
│   ├── chart/                # Workload, scoped RBAC, HTTPRoute and Basic Auth resources
│   ├── values.yaml           # Common resource reservations
│   └── {prd,sandbox}/values.yaml
├── headlamp/             # Headlamp Kubernetes Web UI, in-cluster for prd
│   ├── prd/values.yaml      # hostname
│   └── chart/            # Wrapper chart (in-cluster mode, HTTPRoute)
├── homepage/             # Homepage dashboard (prd, sandbox)
│   ├── {prd,sandbox}/values.yaml  # hostname / Gateway listener per environment
│   └── chart/
└── open-webui/           # Open WebUI values for the upstream chart (prd, AMD GPU)
    ├── values.yaml
    └── prd/values.yaml
```

## Resource Requests

Workloads set **CPU/memory requests without memory limits**. Chart values record
the observations behind each request.

Requests drive both memory-pressure defenses:

- kubelet ranks pods for eviction by how far usage exceeds the request, so the
  container that actually overran is evicted first.
- The kernel OOM score for a Burstable pod is
  `1000 - (1000 * memory request / node capacity)`, so a container with an
  accurate request is protected and one that grew far past its request is the
  first victim.

Limits only cap usage; requests make scheduling and pressure handling accurate.

Observed metrics cannot safely size memory limits. A derived 1 Gi Argo CD
controller limit caused an OOMKill loop because:

- `container_memory_working_set_bytes` is sampled every 30s and misses spikes
  between scrapes. The container that died under the cap recorded a high-water
  mark of only 509Mi.
- The cgroup counts page cache toward the limit while both exported memory
  metrics exclude it, so readings stay low right up to the kill.
- A container that has run uninterrupted for months has never been observed
  through a cold start, so its "peak" describes warm operation only.

Nodes have 28–32 GiB allocatable at about 15% use. Add limits only for measured
leaks and size them from direct evidence.

CPU limits are also omitted because CFS throttles even on idle nodes.

Two exceptions:

- **GPU workloads** (`ollama`, `comfyui`, `lemonade-server`, `vllm`) set
  `limits: amd.com/gpu`, as required for extended-resource allocation.
- **`homepage`, `pdns-ui`, `external-dns`** carry pre-existing CPU and memory
  limits retained from stable production operation.

## Initial Cluster Bootstrap

Bootstrap Argo CD with helmfile, then hand control to App of Apps.

```bash
# Deploy ArgoCD (prd)
cd k8s/argocd/prd
helmfile apply

# Apply root App of Apps
kubectl apply -f k8s/argocd/prd/root-apps.yaml
```

After applying `root-apps.yaml`, use GitOps for subsequent changes.
