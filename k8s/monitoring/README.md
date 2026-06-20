# Monitoring Stack

Monitoring stack for the prd cluster. Managed by ArgoCD with the helm-secrets plugin.

## Components

| Application | Chart | Description |
|-------------|-------|-------------|
| `prometheus` | `kube-prometheus-stack` | Prometheus and Alertmanager with Discord notification routing |
| `loki` | `loki` | Log aggregation, Proxmox log alert ruler, and LoadBalancer ingestion |
| `blackbox-exporter` | `prometheus-blackbox-exporter` | ICMP and DNS probing |
| `snmp-exporter` | `prometheus-snmp-exporter` | SNMP metrics polling |
| `prometheus-pve-exporter` | local chart | Proxmox VE metrics |
| `node-exporter-external` | local chart | Scrape external node-exporter instances |
| `amd-gpu-external` | local chart | AMD GPU metrics (amd-metrics-exporter on GPU VM) |
| `dnsdist` | local chart | dnsdist metrics (Endpoints + ServiceMonitor) |
| `pdns-auth` | local chart | PowerDNS Authoritative metrics |

## Directory Structure

```
monitoring/
├── apps/                         # ArgoCD Application manifests
│   ├── prometheus.yaml
│   ├── loki.yaml
│   ├── blackbox-exporter.yaml
│   ├── snmp-exporter.yaml
│   ├── prometheus-pve-exporter.yaml
│   ├── node-exporter-external.yaml
│   ├── amd-gpu-external.yaml
│   ├── dnsdist.yaml
│   └── pdns-auth.yaml
├── values/                       # Helm values per component
│   ├── prometheus.yaml           # kube-prometheus-stack + Grafana/Prometheus hostnames
│   ├── loki.yaml                 # SingleBinary mode, LoadBalancer
│   ├── blackbox-exporter.yaml
│   ├── snmp-exporter.yaml
│   ├── prometheus-pve-exporter.yaml
│   ├── node-exporter-external.yaml
│   ├── amd-gpu-external.yaml
│   ├── dnsdist.yaml
│   ├── pdns-auth.yaml
│   └── default-values.yaml      # Reference: upstream chart defaults
└── charts/                       # Local Helm charts
    ├── prometheus/               # Wrapper + HTTPRoutes for Grafana and Prometheus
    ├── loki/
    ├── node-exporter-external/
    ├── amd-gpu-external/         # ScrapeConfig for AMD GPU metrics exporter
    ├── dnsdist/
    ├── pdns-auth/
    ├── snmp-exporter/
    └── prometheus-pve-exporter/  # Deployment + ESO ExternalSecret + Probe
```

## Access

| Service | URL | Method |
|---------|-----|--------|
| Grafana | `https://grafana.prd.butaco.net` | HTTPRoute → shared-gateway |
| Prometheus | `https://prometheus.prd.butaco.net` | HTTPRoute → shared-gateway |
| Loki | `loki.prd.butaco.net` (LoadBalancer) | LoadBalancer (external log ingestion) |

> `butaco.net` is a personal domain. Replace it in `values/prometheus.yaml` and `values/loki.yaml`.

Loki uses LoadBalancer intentionally to receive logs from nodes outside the cluster (e.g., Proxmox hosts, VMs).

## Secrets

All secrets are fetched from OpenBao via ESO. They are not stored in this repository.

| OpenBao path | Property | Used by | Description |
|-------------|----------|---------|-------------|
| `k8s/monitoring/grafana` | `adminPassword` | prometheus | Grafana admin password |
| `k8s/monitoring/snmp-exporter` | `community` | snmp-exporter | SNMP community string |
| `k8s/monitoring/alertmanager` | `discord-webhook-url` | Alertmanager | Discord notification webhook |
| `k8s/monitoring/pve-exporter` | `cluster-dev-user` | prometheus-pve-exporter | Proxmox dev API username |
| `k8s/monitoring/pve-exporter` | `cluster-dev-token-name` | prometheus-pve-exporter | Proxmox dev API token name |
| `k8s/monitoring/pve-exporter` | `cluster-dev-token-value` | prometheus-pve-exporter | Proxmox dev API token value |
| `k8s/monitoring/pve-exporter` | `cluster-prd-user` | prometheus-pve-exporter | Proxmox prd API username |
| `k8s/monitoring/pve-exporter` | `cluster-prd-token-name` | prometheus-pve-exporter | Proxmox prd API token name |
| `k8s/monitoring/pve-exporter` | `cluster-prd-token-value` | prometheus-pve-exporter | Proxmox prd API token value |
| `k8s/monitoring/pve-exporter` | `cluster-node2-user` | prometheus-pve-exporter | Proxmox node2 API username |
| `k8s/monitoring/pve-exporter` | `cluster-node2-token-name` | prometheus-pve-exporter | Proxmox node2 API token name |
| `k8s/monitoring/pve-exporter` | `cluster-node2-token-value` | prometheus-pve-exporter | Proxmox node2 API token value |
| `k8s/monitoring/pve-exporter` | `cluster-node3-user` | prometheus-pve-exporter | Proxmox node3 API username |
| `k8s/monitoring/pve-exporter` | `cluster-node3-token-name` | prometheus-pve-exporter | Proxmox node3 API token name |
| `k8s/monitoring/pve-exporter` | `cluster-node3-token-value` | prometheus-pve-exporter | Proxmox node3 API token value |

## Notes

- `serviceMonitorSelectorNilUsesHelmValues: false` — Prometheus discovers all ServiceMonitors cluster-wide
- Target IPs for external exporters (blackbox, node-exporter, etc.) are hardcoded in `values/` files
- `prometheus-pve-exporter` Deployment has a checksum annotation on its Secret for automatic restarts on credential changes
- Future plan: move k0s controller-manager/scheduler scraping into an explicit local chart (`docs/plans/control-plane-metrics-chart.md`)

## Alerting

Alertmanager is the shared notification hub:

- Prometheus evaluates metric alerts from `PrometheusRule` resources.
- The Loki SingleBinary ruler evaluates Proxmox LogQL alerts.
- The initial rollout enables only `ProxmoxAppArmorDenied`; the remaining
  Proxmox rules are enabled incrementally through `values/loki.yaml`.
- Only `warning` and `critical` alerts are routed to Discord.
- `Watchdog`, `InfoInhibitor`, informational alerts, and alerts without a
  supported severity remain on the null receiver.
- Proxmox log alerts are grouped by `alertname` and `host`.
- Resolved notifications are enabled.

The Discord webhook is never stored in Git. ESO reads it from OpenBao into the
`alertmanager-discord` Secret, which Alertmanager mounts at:

```text
/etc/alertmanager/secrets/alertmanager-discord/discord-webhook-url
```

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
ansible-playbook playbooks/openbao_seed_secrets.yaml
```

Do not run `bao kv put` manually; Ansible is the source of truth for seeded
OpenBao values.
