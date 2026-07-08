# sandbox-monitoring

Standalone Prometheus + Grafana for the sandbox cluster. Managed by ArgoCD.

Unlike dev (Prometheus agent remote-writing to prd, see `k8s/dev-monitoring`),
sandbox is intentionally not coupled to the prd monitoring stack: metrics stay
local, Grafana runs in-cluster, and there is no Alertmanager.

## How It Works

- A single wrapper chart deploys `kube-prometheus-stack` in normal (server)
  mode with the bundled Grafana subchart enabled.
- `cluster: sandbox` is stamped at scrape time via a default scrape class so
  cluster-scoped dashboard queries work against the local datasource.
- Alertmanager is disabled; alert rules still evaluate and are visible in the
  Prometheus UI, but nothing is routed anywhere.
- Grafana keeps the upstream default dashboards and has no persistence —
  manually created dashboards do not survive pod restarts.

## Directory Structure

```
sandbox-monitoring/
├── charts/
│   └── prometheus/           # kube-prometheus-stack wrapper (+ Grafana subchart)
│       ├── Chart.yaml
│       └── templates/
│           ├── httproutes.yaml       # Prometheus and Grafana HTTPRoutes (HTTP-only)
│           ├── external-secret.yaml  # Grafana admin credentials from OpenBao
│           ├── argocd.yaml           # Platform scrape resources, kept
│           ├── cilium-servicemonitors.yaml  # semantically paired with the
│           ├── envoy-gateway.yaml    # prd/dev counterparts in
│           └── external-dns.yaml     # k8s/{monitoring,dev-monitoring}
└── values/
    └── prometheus.yaml       # Standalone config, retention, control-plane scraping
```

## Access

| Service | URL | Method |
|---------|-----|--------|
| Grafana | `http://grafana.sandbox.butaco.net` | HTTPRoute → shared-gateway-envoy |
| Prometheus | `http://prometheus.sandbox.butaco.net` | HTTPRoute → shared-gateway-envoy |

Sandbox is HTTP-only (no cert-manager); HTTPRoutes bind to `sectionName: http`.
See ADR-0010.

## Secrets

Grafana admin credentials are fetched from OpenBao via ESO, reusing the same
secret as prd:

| OpenBao path | Property | Used by | Description |
|-------------|----------|---------|-------------|
| `k8s/monitoring/grafana` | `adminPassword` | grafana | Grafana admin password |

Prerequisite: the OpenBao `k8s-eso` role on the sandbox auth mount
(`kubernetes-sandbox`, see `k8s/eso/sandbox/values.yaml`) must carry the
`k8s-monitoring` policy. The policy assignment lives in
`ansible/roles/openbao/defaults/main.yaml` (`openbao_k8s_clusters`); apply it
with `ansible-playbook playbooks/ops-openbao_configure.yaml` before syncing
this application, otherwise the ExternalSecret stays unsynced (and Grafana
pending) until the role can read the secret.

## ArgoCD Application

Wired through the shared app-of-apps chart (`k8s/argocd/apps`); see the
`monitoring` entry in `k8s/argocd/sandbox/apps-values.yaml`:

```yaml
monitoring:
  enabled: true
  path: k8s/sandbox-monitoring/charts/prometheus
  namespace: monitoring
  helm:
    releaseName: prometheus
    valueFiles:
      - ../../values/prometheus.yaml
```

## Key Configuration

| Setting | Value | Description |
|---------|-------|-------------|
| `grafana.enabled` | `true` | Grafana bundled as a subchart (not a separate app like prd) |
| `alertmanager.enabled` | `false` | No notification target on sandbox |
| `retention` | `3d` | Short retention for an experimentation cluster |
| `storageSpec` | 5Gi PVC | Longhorn (cluster default StorageClass) |
| `scrapeClasses` | `cluster-sandbox` (default) | Stamps `cluster=sandbox` on all scraped targets |
| `kubeControllerManager` / `kubeScheduler` | `192.168.20.31` | Dedicated k0s controller scraped by node IP |
| `kubeProxy.enabled` | `false` | Cilium kube-proxy replacement |
