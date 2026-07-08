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
│       ├── Chart.lock
│       └── templates/    # dev-local platform scrape resources
└── values/
    └── prometheus.yaml   # Agent mode config, remoteWrite, resource limits
```

## Key Configuration

| Setting | Value | Description |
|---------|-------|-------------|
| `agentMode` | `true` | Renders a PrometheusAgent instead of a full Prometheus server |
| `remoteWrite.url` | `https://prometheus.prd.butaco.net/api/v1/write` | Remote write target |
| `externalLabels.cluster` | `dev` | Label added to all metrics |
| `grafana.enabled` | `false` | Grafana not deployed on dev |
| `alertmanager.enabled` | `false` | Alertmanager not deployed on dev |
| `resources.requests.memory` | `256Mi` | Reduced memory for agent mode |

## Current Inventory

`k8s/dev-monitoring` is intentionally smaller than `k8s/monitoring`. It does
not own long-term storage, dashboards, Grafana, Alertmanager, Loki, external
LAN target scraping, or notification routing. Its job is to scrape dev-cluster
metrics locally, label them as `cluster=dev`, and remote-write them to the prd
Prometheus.

| Area | Location | Main resources | Classification | Assessment |
|------|----------|----------------|----------------|------------|
| Prometheus Agent | `charts/prometheus` and `values/prometheus.yaml` | `kube-prometheus-stack` in `agentMode`, `remoteWrite`, `externalLabels.cluster=dev`, reduced resources | Dev scrape agent | Correct here. Dev needs a local scraper but prd remains the central backend. |
| Alerting and UI | `values/prometheus.yaml` | `grafana.enabled=false`, `alertmanager.enabled=false` | Delegated to prd | Correct here. Alert evaluation and dashboards are centralized in prd. |
| kubelet and node metrics | `values/prometheus.yaml` | kubelet ServiceMonitor behavior from kube-prometheus-stack, timestamp fixes for agent mode | In-cluster scrape behavior | Correct here. These are dev-local scrape details and remote-write tuning. |
| k0s control-plane metrics | `values/prometheus.yaml` | kube-controller-manager and kube-scheduler endpoints for `192.168.20.11` | Dev control-plane scrape resource | Acceptable here. The controller host is not discoverable as a normal Kubernetes endpoint. |
| ArgoCD metrics | `templates/argocd.yaml` | Four ArgoCD `ServiceMonitor` resources | Bootstrap/cross-cutting scrape resource | Acceptable centralization. Keep paired with the prd counterpart. |
| Cilium and Hubble metrics | `templates/cilium-servicemonitors.yaml` | Cilium agent, Cilium operator, and Hubble `ServiceMonitor` resources | Bootstrap/cross-cutting scrape resource | Acceptable centralization. Cilium is installed before monitoring CRDs exist. |
| Envoy Gateway metrics | `templates/envoy-gateway.yaml` | Controller `ServiceMonitor` and proxy `PodMonitor` | Bootstrap/cross-cutting scrape resource | Acceptable centralization. Keep paired with the prd counterpart because selector and pod metrics behavior should stay consistent. |
| external-dns metrics | `templates/external-dns.yaml` | `ServiceMonitor` for external-dns | Cross-cutting scrape resource | Acceptable here for parity with prd. It could be app-owned later if CRD ordering and env parity remain clear. |

Current policy fit:

- This chart may contain dev-cluster scrape wiring needed for remote_write.
- It should not gain Grafana, Alertmanager, Loki, dashboards, external LAN
  target scraping, or notification routing unless dev intentionally stops being
  a thin agent.
- Any platform scrape template duplicated from prd should stay semantically
  paired with `k8s/monitoring/charts/prometheus/templates/`.
- If an app can safely own its own `ServiceMonitor` after Prometheus Operator
  CRDs exist, prefer moving that scrape resource to the app instead of growing
  this wrapper.

## ArgoCD Application

```yaml
# k8s/argocd/dev/apps-values.yaml
apps:
  monitoring:
    enabled: true
    path: k8s/dev-monitoring/charts/prometheus
    namespace: monitoring
    helm:
      releaseName: prometheus
      valueFiles:
        - ../../values/prometheus.yaml
    syncOptions:
      - CreateNamespace=true
      - ServerSideApply=true
```
