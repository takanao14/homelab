# Monitoring Stack

Monitoring stack for the prd cluster. Managed by ArgoCD with the helm-secrets plugin.

## Components

| Application | Chart | Description |
|-------------|-------|-------------|
| `prometheus` | `kube-prometheus-stack` | Prometheus, Grafana, Alertmanager |
| `loki` | `loki` | Log aggregation (LoadBalancer for external log ingestion) |
| `blackbox-exporter` | `prometheus-blackbox-exporter` | ICMP and DNS probing |
| `snmp-exporter` | `prometheus-snmp-exporter` | SNMP metrics polling |
| `prometheus-pve-exporter` | local chart | Proxmox VE metrics |
| `node-exporter-external` | local chart | Scrape external node-exporter instances |
| `dnsdist` | local chart | dnsdist metrics (Endpoints + ServiceMonitor) |
| `pdns-auth` | local chart | PowerDNS Authoritative metrics |

## Directory Structure

```
monitoring/
├── secrets.enc.yaml              # SOPS-encrypted secrets
├── apps/                         # ArgoCD Application manifests
│   ├── prometheus.yaml
│   ├── loki.yaml
│   ├── blackbox-exporter.yaml
│   ├── snmp-exporter.yaml
│   ├── prometheus-pve-exporter.yaml
│   ├── node-exporter-external.yaml
│   ├── dnsdist.yaml
│   └── pdns-auth.yaml
├── values/                       # Helm values per component
│   ├── prometheus.yaml           # kube-prometheus-stack + Grafana/Prometheus hostnames
│   ├── loki.yaml                 # SingleBinary mode, LoadBalancer
│   ├── blackbox-exporter.yaml
│   ├── snmp-exporter.yaml
│   ├── prometheus-pve-exporter.yaml
│   ├── node-exporter-external.yaml
│   ├── dnsdist.yaml
│   ├── pdns-auth.yaml
│   └── default-values.yaml      # Reference: upstream chart defaults
└── charts/                       # Local Helm charts
    ├── prometheus/               # Wrapper + HTTPRoutes for Grafana and Prometheus
    ├── loki/
    ├── node-exporter-external/
    ├── dnsdist/
    ├── pdns-auth/
    ├── snmp-exporter/
    └── prometheus-pve-exporter/  # Checksum annotation for auto-restart on Secret change
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

```bash
sops edit k8s/monitoring/secrets.enc.yaml
```

| Variable | Used by | Description |
|----------|---------|-------------|
| `grafana.adminPassword` | prometheus | Grafana admin password |
| `snmp.community` | snmp-exporter | SNMP community string |
| `proxmox.prd.user` | prometheus-pve-exporter | Proxmox prd API username |
| `proxmox.prd.tokenName` | prometheus-pve-exporter | Proxmox prd API token name |
| `proxmox.prd.tokenValue` | prometheus-pve-exporter | Proxmox prd API token value |
| `proxmox.dev.user` | prometheus-pve-exporter | Proxmox dev API username |
| `proxmox.dev.tokenName` | prometheus-pve-exporter | Proxmox dev API token name |
| `proxmox.dev.tokenValue` | prometheus-pve-exporter | Proxmox dev API token value |
| `proxmox.prd2.user` | prometheus-pve-exporter | Proxmox prd2 API username |
| `proxmox.prd2.tokenName` | prometheus-pve-exporter | Proxmox prd2 API token name |
| `proxmox.prd2.tokenValue` | prometheus-pve-exporter | Proxmox prd2 API token value |

## Notes

- `serviceMonitorSelectorNilUsesHelmValues: false` — Prometheus discovers all ServiceMonitors cluster-wide
- Target IPs for external exporters (blackbox, node-exporter, etc.) are hardcoded in `values/` files
- `prometheus-pve-exporter` Deployment has a checksum annotation on its Secret for automatic restarts on credential changes
