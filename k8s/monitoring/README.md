# Monitoring Stack

Monitoring stack for the homelab Kubernetes cluster, deployed via Helmfile.

## Components

| Release | Chart | Description |
|---------|-------|-------------|
| `prometheus` | `kube-prometheus-stack` | Prometheus, Grafana, Alertmanager |
| `blackbox-exporter` | `prometheus-blackbox-exporter` | ICMP and DNS probing |
| `snmp-exporter` | `prometheus-snmp-exporter` | SNMP metrics polling |
| `prometheus-pve-exporter` | local chart | Proxmox VE metrics |
| `node-exporter-external` | local chart | Scrape external node-exporter instances |
| `dnsdist` | local chart | dnsdist metrics |
| `pdns-auth` | local chart | PowerDNS Authoritative metrics |

## Directory Structure

```
monitoring/
├── helmfile.yaml
├── secrets.enc.env              # SOPS-encrypted secrets (committed)
├── .envrc                       # Decrypts secrets (gitignored)
├── values/
│   ├── prometheus.yaml.gotmpl
│   ├── blackbox-exporter.yaml.gotmpl
│   ├── snmp-exporter.yaml.gotmpl
│   ├── node-exporter.yaml.gotmpl
│   ├── dnsdist.yaml.gotmpl
│   ├── pdns-auth.yaml.gotmpl
│   ├── prometheus-pve-exporter.yaml.gotmpl
│   └── default-values.yaml      # Reference: default values from upstream charts
└── charts/
    ├── node-exporter-external/
    ├── dnsdist/
    ├── pdns-auth/
    └── prometheus-pve-exporter/ # Includes checksum annotation for auto-restart on Secret change
```

## Deployment

### Prerequisites

- Kubernetes cluster with `helmfile`, `kubectl`
- `sops` + `direnv` for secret management

### 1. Set up secrets

```bash
cd k8s/monitoring
sops edit secrets.enc.env
direnv allow
```

### 2. Deploy

```bash
helmfile apply
```

## Secret Variables

| Variable | Used by | Description |
|----------|---------|-------------|
| `GRAFANA_ADMIN_PASSWORD` | prometheus (Grafana) | Grafana admin password |
| `PROXMOX_PRD_USER` | prometheus-pve-exporter | Proxmox VE API username |
| `PROXMOX_PRD_PASSWORD` | prometheus-pve-exporter | Proxmox VE API password |

Non-sensitive values (IPs, hostnames, ports) are hardcoded directly in the `values/*.yaml.gotmpl` files.

## Accessing Dashboards

- **Grafana**: `http://grafana.prd.butaco.net` (LoadBalancer via ExternalDNS)
- **Prometheus**: `http://prometheus.prd.butaco.net` (LoadBalancer via ExternalDNS)

Admin credentials are managed by the Helm release. The Grafana password is set via `GRAFANA_ADMIN_PASSWORD`.

## Notes

- The `prometheus-pve-exporter` Deployment has a checksum annotation on its Secret, so it automatically restarts when the Secret changes (e.g., after `helmfile apply` with updated credentials).
- Target IPs for external exporters (blackbox, node-exporter, etc.) are hardcoded in values files.
