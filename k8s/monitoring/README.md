# Monitoring Stack

Monitoring stack for the prd cluster. Managed by ArgoCD with the helm-secrets plugin.

## Components

| Application | Chart | Description |
|-------------|-------|-------------|
| `prometheus` | `kube-prometheus-stack` | Prometheus and Alertmanager with Discord notification routing |
| `grafana` | `grafana` | Grafana UI, datasources, dashboard sidecar, and HTTPRoute |
| `loki` | `loki` | Log aggregation, Proxmox log alert ruler, and LoadBalancer ingestion |
| `alloy` | `alloy` | OTLP ingestion for Proxmox metrics and remote_write into Prometheus |
| `blackbox-exporter-external` | local chart | ScrapeConfigs for ICMP and DNS probes through an external blackbox_exporter |
| `snmp-exporter` | `prometheus-snmp-exporter` | SNMP metrics polling |
| `node-exporter-external` | local chart | Scrape external node-exporter instances |
| `amd-gpu-external` | local chart | AMD GPU metrics (amd-metrics-exporter on GPU VM) |
| `dnsdist` | local chart | dnsdist external scrape configuration |
| `pdns-auth` | local chart | PowerDNS Authoritative metrics |
| `openbao` | local chart | OpenBao external scrape configuration |

## Directory Structure

```
monitoring/
├── apps/                         # ArgoCD Application manifests
│   ├── prometheus.yaml
│   ├── grafana.yaml
│   ├── loki.yaml
│   ├── alloy.yaml
│   ├── blackbox-exporter-external.yaml
│   ├── snmp-exporter.yaml
│   ├── node-exporter-external.yaml
│   ├── amd-gpu-external.yaml
│   ├── dnsdist.yaml
│   ├── pdns-auth.yaml
│   └── openbao.yaml
├── values/                       # Helm values per component
│   ├── prometheus.yaml           # kube-prometheus-stack + Prometheus/Alertmanager config
│   ├── grafana.yaml
│   ├── loki.yaml                 # SingleBinary mode, LoadBalancer
│   ├── alloy.yaml
│   ├── blackbox-exporter-external.yaml
│   ├── snmp-exporter.yaml
│   ├── node-exporter-external.yaml
│   ├── amd-gpu-external.yaml
│   ├── dnsdist.yaml
│   ├── pdns-auth.yaml
│   └── openbao.yaml
└── charts/                       # Local Helm charts
    ├── prometheus/               # kube-prometheus-stack wrapper + shared scrape/rule resources
    ├── grafana/
    ├── loki/
    ├── alloy/
    ├── node-exporter-external/
    ├── amd-gpu-external/         # ScrapeConfig for AMD GPU metrics exporter
    ├── dnsdist/
    ├── pdns-auth/
    ├── openbao/
    └── snmp-exporter/
```

## Ownership Policy

`k8s/monitoring` is the prd observability control plane, not just the
Prometheus chart. Keep monitoring-owned resources here when their primary
purpose is to collect, store, query, alert on, or expose telemetry.

It is acceptable for this directory to be large: observability is naturally
cross-cutting, and keeping the scrape contracts, dashboards, alerting, and
telemetry backends together makes operations easier to review. The constraint
is that the directory must grow as small, responsibility-focused charts and
values files, not as an unbounded `charts/prometheus` dumping ground.

Use these boundaries:

- Telemetry backends and frontends belong here: Prometheus, Alertmanager,
  Grafana, Loki, Alloy, their Secrets, HTTPRoutes, LoadBalancers, dashboards,
  and alert routing.
- Exporter workloads belong here only when the exporter exists solely as
  monitoring infrastructure and is not part of a product workload. Examples:
  `snmp-exporter` and in-cluster collectors/receivers such as Alloy.
- External exporter targets stay here as small local charts when Kubernetes
  only owns the scrape contract (`ScrapeConfig`, `Probe`, `ServiceMonitor`, or
  related ESO Secret), while the exporter process runs outside the cluster.
  Examples: `node-exporter-external`, `amd-gpu-external`, `dnsdist`,
  `pdns-auth`, `openbao`, and blackbox probes through the rpi4 exporter.
- Application-owned metrics endpoints should stay with the owning application
  when that application is deployed by ArgoCD after the monitoring CRDs exist.
  The application chart may create its own metrics Service, ServiceMonitor,
  PodMonitor, or PrometheusRule if doing so does not break bootstrap ordering.
- Bootstrap or cross-cutting scrape resources may stay in
  `charts/prometheus/templates/` when the monitored component is installed
  before Prometheus Operator CRDs exist, or when the same scrape policy must be
  centralized across clusters. Current examples are Cilium, Envoy Gateway,
  ArgoCD, kube control-plane scraping, Loki self-scraping, and external-dns.
- Dashboards and alert rules belong with the telemetry backend that loads or
  evaluates them unless a rule is tightly coupled to a workload chart. Grafana
  dashboards currently live under the Prometheus wrapper because the Grafana
  dashboard sidecar discovers labeled ConfigMaps across namespaces.

The main tradeoff is intentional centralization versus ownership clarity. This
directory centralizes monitoring-specific resources so bootstrap ordering,
Prometheus Operator selectors, common labels, and dashboards remain coherent.
To preserve ownership clarity, do not move application runtime configuration or
product deployments here just because they expose metrics. When a monitored app
can safely own its own metrics objects, keep those objects with the app.

Adding a new monitoring item:

1. If it deploys a reusable exporter process, prefer an upstream chart wrapper
   or a dedicated local chart under `charts/<name>/`.
2. If it only describes external scrape targets, create a small local chart
   that renders only the monitoring CRDs and keep target IPs in
   `values/<name>.yaml`.
3. If it monitors an app that can own its own metrics objects safely, put the
   monitoring objects in that app's chart instead of growing
   `charts/prometheus`.
4. If it must stay centralized because of CRD ordering or cross-cluster
   semantics, document that reason in the template comment.

Avoid putting application deployments, product-specific runtime configuration,
or non-telemetry infrastructure here merely because they expose metrics. Also
avoid adding unrelated scrape resources directly to `charts/prometheus`; prefer
a dedicated small chart unless the resource genuinely belongs to the Prometheus
control plane or must be centralized for bootstrap/cross-cluster reasons.

## Current Inventory

This inventory records why each current component lives here and whether it is
a good fit for the ownership policy above.

| Area | Location | Main resources | Classification | Assessment |
|------|----------|----------------|----------------|------------|
| Prometheus and Alertmanager | `charts/prometheus` | `kube-prometheus-stack`, Prometheus, Alertmanager, default selectors, Prometheus HTTPRoute | Telemetry backend and control plane | Correct here. Keep Prometheus runtime and Alertmanager routing centralized. |
| Grafana | `charts/grafana` | Grafana deployment, admin `ExternalSecret`, HTTPRoute, datasource and dashboard sidecar values | Telemetry frontend | Correct here. Grafana is shared observability UI, not app-owned. |
| Loki | `charts/loki` | Loki SingleBinary, LoadBalancer ingestion, Proxmox LogQL ruler ConfigMap | Telemetry backend and log alerting | Correct here. Log ingestion and alert evaluation are observability control-plane concerns. |
| Alloy | `charts/alloy` | Alloy deployment, OTLP LoadBalancer Service, HTTPRoute | Telemetry receiver/collector | Correct here. It exists to receive Proxmox OTLP metrics and remote_write them into Prometheus. |
| SNMP exporter | `charts/snmp-exporter` | `prometheus-snmp-exporter`, SNMP auth `ExternalSecret`, network-device `Probe` via chart values | Monitoring-only exporter workload | Correct here. The exporter exists solely as monitoring infrastructure. |
| External node exporter targets | `charts/node-exporter-external` | `ScrapeConfig` for Proxmox nodes, Raspberry Pis, and stateful service guests | External scrape contract | Correct here. Kubernetes owns only the scrape contract; exporters run outside the cluster. |
| AMD GPU exporter target | `charts/amd-gpu-external` | `ScrapeConfig` for the GPU VM exporter | External scrape contract | Correct here. The GPU exporter runs outside this cluster and is consumed centrally. |
| Blackbox probes | `charts/blackbox-exporter-external` | ICMP and DNS `ScrapeConfig` resources through the rpi4 blackbox_exporter | External probe contract | Correct here. The probing endpoint is external; monitoring owns probe definitions and target labels. |
| dnsdist metrics | `charts/dnsdist` | `ScrapeConfig` for external dnsdist instances | External scrape contract | Correct here. DNS service runtime stays outside this chart; monitoring owns scrape configuration. |
| PowerDNS Authoritative metrics | `charts/pdns-auth` | `ScrapeConfig` for external authoritative DNS instances | External scrape contract | Correct here. DNS service runtime stays outside this chart; monitoring owns scrape configuration. |
| OpenBao metrics | `charts/openbao` | `ScrapeConfig` for the external OpenBao VM | External scrape contract | Correct here. OpenBao is not deployed here; only its scrape contract is. |
| Dashboards | `charts/prometheus/dashboards`, `dashboards/` | Generated Grafana dashboard JSON and ConfigMaps labeled for the Grafana sidecar | Dashboard delivery | Acceptable here for now because Grafana discovers dashboard ConfigMaps via labels. If dashboard volume grows further, consider a dedicated `dashboards` chart. |
| Shared platform ServiceMonitors | `charts/prometheus/templates/{argocd,cilium-servicemonitors,envoy-gateway,external-dns}.yaml` | `ServiceMonitor` and `PodMonitor` resources for bootstrap/platform apps | Bootstrap/cross-cutting scrape resources | Acceptable centralization. These have CRD ordering or cross-cutting consistency reasons; keep comments explaining why they are not app-owned. |
| Control-plane node exporter | `charts/prometheus/templates/control-plane-node-exporter.yaml` | `ScrapeConfig` for k0s controller host node-exporter targets | Bootstrap/control-plane scrape resource | Acceptable but a future split candidate. It is centralized because k0s control-plane hosts are not discoverable as normal Kubernetes nodes. |
| Metric alert rules | `charts/prometheus/templates/{cert-manager-rules,hardware-rules}.yaml` | `PrometheusRule` resources | Central alert evaluation | Acceptable. Rules evaluate in the central prd Prometheus. App-specific rules can move to app charts once ownership and CRD ordering are safe. |
| Loki self-scrape | `charts/prometheus/templates/servicemonitor-loki.yaml` | `ServiceMonitor` for Loki | Backend self-monitoring | Acceptable. Could move to the Loki wrapper chart if chart sync ordering and selector behavior remain safe. |

Near-term cleanup candidates:

1. Keep `charts/prometheus` from growing further by default. New external
   targets should get their own small chart unless they are part of Prometheus
   control-plane behavior.
2. Consider moving `servicemonitor-loki.yaml` into `charts/loki` if the Loki
   app can safely depend on the Prometheus Operator CRDs at sync time.
3. Consider a dedicated dashboard delivery chart if dashboard ConfigMaps become
   the dominant source of churn in the Prometheus wrapper.
4. Keep prd/dev duplicated platform scrape templates intentionally paired; when
   changing ArgoCD, Cilium, Envoy Gateway, or external-dns scraping, update
   `k8s/dev-monitoring` at the same time.

## Access

| Service | URL | Method |
|---------|-----|--------|
| Grafana | `https://grafana.prd.butaco.net` | HTTPRoute → shared-gateway-envoy |
| Prometheus | `https://prometheus.prd.butaco.net` | HTTPRoute → shared-gateway-envoy |
| Loki | `loki.prd.butaco.net` (LoadBalancer) | LoadBalancer (external log ingestion) |

> `butaco.net` is a personal domain. Replace it in `values/prometheus.yaml` and `values/loki.yaml`.

Loki uses LoadBalancer intentionally to receive logs from nodes outside the cluster (e.g., Proxmox hosts, VMs).

## Secrets

All secrets are fetched from OpenBao via ESO. They are not stored in this repository.

| OpenBao path | Property | Used by | Description |
|-------------|----------|---------|-------------|
| `k8s/monitoring/grafana` | `adminPassword` | grafana | Grafana admin password |
| `k8s/monitoring/snmp-exporter` | `community` | snmp-exporter | SNMP community string |
| `k8s/monitoring/alertmanager` | `discord-webhook-url` | Alertmanager | Discord notification webhook |

## Notes

- `serviceMonitorSelectorNilUsesHelmValues: false` — Prometheus discovers all ServiceMonitors cluster-wide
- Target IPs for external exporters (blackbox, node-exporter, etc.) are hardcoded in `values/` files
- Future plan: move k0s controller-manager/scheduler scraping into an explicit local chart (`docs/plans/control-plane-metrics-chart.md`)

## Alerting

Alertmanager is the shared notification hub:

- Prometheus evaluates metric alerts from `PrometheusRule` resources.
- The Loki SingleBinary ruler evaluates Proxmox LogQL alerts.
- All seven Proxmox LogQL rules are enabled through `values/loki.yaml`
  (`ProxmoxAppArmorDenied`, `ProxmoxOOMDetected`, `ProxmoxStorageError`,
  `ProxmoxBackupFailed`, `ProxmoxServiceErrors`, `ProxmoxQuorumError`,
  `ProxmoxHAError`).
- Only `warning` and `critical` alerts are routed to Discord.
- `Watchdog`, `InfoInhibitor`, informational alerts, and alerts without a
  supported severity remain on the null receiver.
- Proxmox log alerts are grouped by `alertname` and `host`.
- Resolved notifications are enabled.

The Discord webhook is never stored in Git. ESO reads it from OpenBao into the
`alertmanager-discord` Secret. An `AlertmanagerConfig` references its
`discord-webhook-url` key through a `SecretKeySelector`, and the Prometheus
Operator generates the runtime Alertmanager configuration.

Before enabling the receiver in a live cluster, create the Discord webhook and
add it to the encrypted Ansible `openbao_secrets` list:

```yaml
- path: secret/k8s/monitoring/alertmanager
  data:
    discord-webhook-url: "<Discord webhook URL>"
```

Edit the file through SOPS and seed OpenBao with Ansible:

```bash
sops ansible/inventories/homelab/group_vars/openbao.sops.yaml
cd ansible
ansible-playbook playbooks/ops-openbao_seed_secrets.yaml
```

Do not run `bao kv put` manually; Ansible is the source of truth for seeded
OpenBao values.
