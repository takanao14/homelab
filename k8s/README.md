# k8s

Kubernetes manifests and Helm charts for homelab clusters managed via ArgoCD GitOps.

## Environments

| Environment | Domain | Cluster |
|-------------|--------|---------|
| prd | `*.prd.butaco.net` | prd-homelab |
| sandbox | `*.sandbox.butaco.net` (HTTP only) | sandbox-homelab |

> Replace the personal `butaco.net` domain throughout `k8s/` before reuse.

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

- ESO reads OpenBao KV v2 through Kubernetes authentication.
- The `eso` chart creates `ClusterSecretStore/openbao`.
- Ansible manages OpenBao under `ansible/roles/openbao`.

## Directory Structure

```
k8s/
├── argocd/           # Self-management and App of Apps
├── cert-manager/     # Wildcard certificates and controller values
├── envoy-gateway/    # Controller and sole Gateway API CRD owner
├── eso/              # External Secrets Operator and OpenBao store
├── externalDNS/      # PowerDNS record reconciliation
├── gateway/          # Shared Gateway API resources
├── monitoring/       # Metrics, logs, exporters, and dashboard generator
├── reloader/         # Secret/ConfigMap-triggered restarts
├── gpu-switch/       # Exclusive GPU workload controller
├── comfyui/          # GPU image generation
├── lemonade-server/  # GPU LLM inference
├── ollama/           # GPU LLM inference
├── vllm/             # OpenAI-compatible GPU inference
├── open-webui/       # LLM frontend
├── pdns-ui/          # Read-only PowerDNS UI
├── headlamp/         # Kubernetes UI
└── homepage/         # Service dashboard
```

Most applications contain a local `chart/` plus shared and per-environment
values. See each directory's README for details.

## Resource Requests

Workloads normally set measured CPU/memory requests without limits. Requests
drive scheduling, eviction priority, and OOM scoring; sampled metrics can miss
short spikes and page cache, making them unsafe limit inputs. Add limits only
for demonstrated leaks. CPU limits are omitted to avoid CFS throttling.

Two exceptions:

- GPU workloads set `limits: amd.com/gpu` for device allocation.
- `homepage`, `pdns-ui`, and `external-dns` retain established CPU/memory limits.

## Initial Cluster Bootstrap

Bootstrap Argo CD with helmfile, then hand control to App of Apps.

```bash
# Bootstrap prd
cd k8s/argocd/prd
helmfile apply

kubectl apply -f k8s/argocd/prd/root-apps.yaml
```

After applying `root-apps.yaml`, use GitOps for subsequent changes.
