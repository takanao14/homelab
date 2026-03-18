# Monitoring Stack

This directory contains the monitoring stack configuration for the homelab Kubernetes cluster.

## Components

- **kube-prometheus-stack**: Prometheus, Grafana, and Alertmanager deployed via Helm
- **blackbox-exporter**: ICMP and DNS probing metrics deployed via Helm
- **snmp-exporter**: SNMP metrics polling deployed via Helm
- **prometheus-pve-exporter**: Proxmox VE metrics polling deployed via local Helm chart
- **node-exporter (external)**: Scraping node-exporter running on external nodes via local Helm chart
- **dnsdist**: DNS load balancer metrics via local Helm chart
- **pdns-auth**: PowerDNS Authoritative metrics via local Helm chart

## Directory Structure

```
monitoring/
├── helmfile.yaml                # Main deployment configuration
├── .envrc.example               # Template for environment variables
├── values/                      # Helm values for each release
│   ├── prometheus.yaml.gotmpl
│   ├── blackbox-exporter.yaml.gotmpl
│   ├── snmp-exporter.yaml.gotmpl
│   ├── node-exporter.yaml.gotmpl
│   ├── dnsdist.yaml.gotmpl
│   ├── pdns-auth.yaml.gotmpl
│   ├── prometheus-pve-exporter.yaml.gotmpl
│   └── default-values.yaml      # Reference: default values from upstream charts
└── charts/                      # Local Helm charts for custom exporters
    ├── node-exporter-external/  # Service and ServiceMonitor for external node-exporter
    ├── dnsdist/                 # Service and ServiceMonitor for dnsdist
    ├── pdns-auth/               # Service and ServiceMonitor for PowerDNS Auth
    └── prometheus-pve-exporter/ # Deployment, Secret, and Probe for PVE Exporter
```

## Deployment

### Prerequisites

- Kubernetes cluster
- `helmfile` installed ([installation guide](https://github.com/helmfile/helmfile))
- `kubectl` configured with appropriate context
- `direnv` available (for environment variable injection)

### 1. Configure Environment Variables

Copy and edit the environment variables file with your actual credentials and target IPs:

```bash
cd k8s/monitoring
cp .envrc.example .envrc
vi .envrc
direnv allow
```

**Note:** Keep `k8s/monitoring/.envrc` local only; never commit credentials. Use `.envrc.example` as the tracked template.

### 2. Deploy All Releases

Deploy the entire monitoring stack with a single command:

```bash
helmfile apply
```

This will:
- Add necessary Helm repositories
- Create the `monitoring` namespace if it doesn't exist
- Install or upgrade all releases (kube-prometheus-stack, blackbox-exporter, snmp-exporter, and all local charts)
- Inject credentials and target IPs from environment variables via `.gotmpl` values files

### Accessing the Dashboards

Access Grafana via LoadBalancer/Ingress (depending on your cluster setup):
- URL: `http://grafana.k8s.homelab.internal` (or similar depending on your DNS)
- The admin password is managed by the Helm release (check secrets or predefined values).

Access Prometheus:
- URL: `http://prometheus.k8s.homelab.internal` (or similar depending on your DNS)
