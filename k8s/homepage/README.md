# Homepage

[Homepage](https://gethomepage.dev/) dashboard deployed on the homelab Kubernetes cluster.

## Directory Structure

```
homepage/
├── helmfile.yaml                  # Helmfile entrypoint (environments: dev, prd)
├── values-common.yaml.gotmpl      # Common values (credentials via requiredEnv)
├── values-dev.yaml.gotmpl         # Dev environment overrides
├── values-prd.yaml.gotmpl         # Prd environment overrides
├── secrets.enc.env                # SOPS-encrypted secrets (committed)
├── .envrc                         # Decrypts secrets (gitignored)
└── chart/                         # Custom Helm chart
    ├── Chart.yaml
    ├── values.yaml
    └── templates/
        ├── _config.tpl            # Homepage config templates (services, widgets, etc.)
        ├── secret-config.yaml     # Secret rendered from _config.tpl
        ├── deployment.yaml
        ├── service.yaml           # LoadBalancer with ExternalDNS annotation
        └── rbac.yaml
```

## Deployment

### 1. Set up secrets

```bash
cd k8s/homepage
sops edit secrets.enc.env
direnv allow
```

### 2. Apply

```bash
# Production
helmfile -e prd apply

# Development
helmfile -e dev apply

# Dry-run (diff)
helmfile -e prd diff
```

## Configuration

Homepage configuration (`services.yaml`, `widgets.yaml`, `settings.yaml`, etc.) is generated from Go templates in `chart/templates/_config.tpl` and embedded into a Kubernetes Secret (`homepage-config`).

Non-sensitive service URLs and IPs are hardcoded in the chart templates. Credentials are injected via `requiredEnv`.

## Secret Variables

| Variable | Description |
|----------|-------------|
| `PROXMOX_PRD_USERNAME` | Proxmox VE username (production) |
| `PROXMOX_PRD_PASSWORD` | Proxmox VE password/token (production) |
| `PROXMOX_DEV_USERNAME` | Proxmox VE username (development) |
| `PROXMOX_DEV_PASSWORD` | Proxmox VE password/token (development) |
| `TRUENAS_API_KEY` | TrueNAS API key |
| `GRAFANA_PASSWORD` | Grafana admin password |

## Services Displayed

- **DNS**: PowerDNS auth1/auth2, dnsdist1/dnsdist2
- **VM/Storage**: Proxmox VE (Prd/Dev), TrueNAS
- **Monitoring**: Grafana, Prometheus
- **Develop**: Forgejo
- **Network**: Border Gateway, L3-SW, WiFi APs
