# dev-monitoring

Prometheus in agent mode for the dev cluster. Scrapes metrics locally and remote-writes to the prd Prometheus instance. Managed by ArgoCD.

## How It Works

- Prometheus runs in [agent mode](https://prometheus.io/docs/prometheus/latest/feature_flags/#prometheus-agent) — it scrapes and remote-writes but does not store data locally.
- All scraped metrics are forwarded to `https://prometheus.prd.butaco.net/api/v1/write`.
- External labels `cluster: dev` are added so metrics can be distinguished in the prd Prometheus.
- Grafana and Alertmanager are disabled (managed from prd).

## Directory Structure

```
dev-monitoring/
├── charts/
│   └── prometheus/       # kube-prometheus-stack wrapper (agent mode)
│       ├── Chart.yaml
│       └── Chart.lock
└── values/
    └── prometheus.yaml   # Agent mode config, remoteWrite, resource limits
```

## Key Configuration

| Setting | Value | Description |
|---------|-------|-------------|
| `enableFeatures` | `agent` | Enables Prometheus agent mode |
| `remoteWrite.url` | `https://prometheus.prd.butaco.net/api/v1/write` | Remote write target |
| `externalLabels.cluster` | `dev` | Label added to all metrics |
| `grafana.enabled` | `false` | Grafana not deployed on dev |
| `alertmanager.enabled` | `false` | Alertmanager not deployed on dev |
| `resources.requests.memory` | `256Mi` | Reduced memory for agent mode |

## ArgoCD Application

```yaml
# k8s/argocd/dev/apps/monitoring.yaml
source:
  path: k8s/dev-monitoring/charts/prometheus
  helm:
    releaseName: prometheus
    valueFiles:
      - ../../values/prometheus.yaml
destination:
  namespace: monitoring
```
