# k8s

Kubernetes manifests and Helm charts for homelab clusters managed via ArgoCD GitOps.

## Environments

| Environment | Domain | Cluster |
|-------------|--------|---------|
| prd | `*.prd.butaco.net` | prd-homelab |
| sandbox | `*.sandbox.butaco.net` (HTTP only) | sandbox-homelab |

There is no staging environment. sandbox is a disposable cluster for platform
changes only and never reads prd credentials
([ADR-0070](../docs/adr/0070-no-staging-sandbox-validates-the-cluster-platform.md)).

> Replace the personal `butaco.net` domain throughout `k8s/` before reuse.

## Architecture

### Networking

- **CNI**: Cilium, pinned in `k0s/helmfile.yaml.gotmpl`
- **Ingress**: Envoy Gateway via Gateway API; `envoy-gateway/crds/` pins the CRDs
- **TLS**: cert-manager wildcard certificate via Cloudflare DNS-01 challenge
  for prd; sandbox intentionally uses HTTP without cert-manager
- **DNS**: external-dns with PowerDNS provider (`gateway-httproute` source) for
  prd; sandbox uses a dnscontrol wildcard to its pinned Gateway address

HTTPRoutes use `gateway-system/shared-gateway-envoy`. See
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
├── mcp/              # Authenticated Grafana and NetBox MCP servers (prd)
├── reloader/         # Secret/ConfigMap-triggered restarts
├── gpu-switch/       # Exclusive GPU workload controller
├── comfyui/          # GPU image generation
├── lemonade-server/  # GPU LLM inference
├── butaco-voice/     # Voice chat and VOICEVOX CPU engine
├── ollama/           # GPU LLM inference
├── vllm/             # OpenAI-compatible GPU inference
├── open-webui/       # LLM frontend
├── pdns-ui/          # Read-only PowerDNS UI
├── headlamp/         # Kubernetes UI
├── slack-bot/        # Slack bot rendering to the LED matrix
└── homepage/         # Service dashboard
```

Most applications contain a local `chart/` plus shared and per-environment
values. See each directory's README for details.

## Resource Requests

Workloads normally set measured CPU/memory requests without limits. Requests
drive scheduling, eviction priority, and OOM scoring; sampled metrics can miss
short spikes and page cache, making them unsafe limit inputs. Add limits only
for demonstrated leaks. CPU limits are omitted to avoid CFS throttling.

Exceptions:

- GPU workloads set `limits: amd.com/gpu` for device allocation.
- `homepage` retains a memory limit; `pdns-ui` and `external-dns` retain
  CPU and memory limits.

## Initial Cluster Bootstrap

Follow [Argo CD initial deployment](argocd/README.md#initial-deployment) to
bootstrap with Helmfile and hand ownership to App of Apps. Subsequent changes
go through GitOps.
