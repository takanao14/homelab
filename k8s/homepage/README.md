# Homepage

[Homepage](https://gethomepage.dev/) is a modern, fully static, fast, secure fully customizable application dashboard with integrations for over 100 services and translations into over 40 languages.

## Directory Structure

```
.
├── helmfile.yaml           # Helmfile entrypoint (environments: dev, prd)
├── values-dev.yaml         # Dev environment overrides
├── values-prd.yaml         # Prd environment overrides
└── chart/                  # Custom Helm chart
    ├── Chart.yaml
    ├── values.yaml         # Default values
    └── templates/
        ├── _config.tpl     # Homepage config templates (services, widgets, etc.)
        ├── secret-config.yaml  # ConfigMap rendered from _config.tpl
        ├── deployment.yaml
        ├── service.yaml    # LoadBalancer with external-dns annotation
        └── rbac.yaml
```

## Deployment

```bash
# Apply to production
helmfile -e prd apply

# Apply to development
helmfile -e dev apply

# Dry-run (diff)
helmfile -e prd diff
```

## Configuration

Homepage configuration files (`services.yaml`, `widgets.yaml`, `settings.yaml`, `proxmox.yaml`, `kubernetes.yaml`) are generated from Go templates in `chart/templates/_config.tpl` and embedded into a Kubernetes `Secret` (`homepage-config`) via `chart/templates/secret-config.yaml`.

### Values

Default values are defined in `chart/values.yaml`. Environment-specific overrides are in `values-dev.yaml` and `values-prd.yaml`.

| Key | Description |
|-----|-------------|
| `hostname` | ExternalDNS hostname for the LoadBalancer Service |
| `proxmox.prd` / `proxmox.dev` | Proxmox VE connection info |
| `pdns.primary` / `pdns.secondary` | PowerDNS authoritative server info |
| `dnsdist.dnsdist1` / `dnsdist.dnsdist2` | dnsdist load balancer info |
| `truenas` | TrueNAS connection info |
| `grafana` / `prometheus` | Monitoring service info |
| `forgejo` | Forgejo (Git) URL |
| `network` | Network device addresses |

> **Note:** `values-*.yaml` may contain sensitive data (passwords, API keys) and must not be committed to the repository.

## Services Displayed

- **DNS**: PowerDNS auth1/auth2, dnsdist1/dnsdist2
- **VM/Storage**: Proxmox VE (Prd/Dev), TrueNAS
- **Monitoring**: Grafana, Prometheus
- **Develop**: Forgejo
- **Network**: Border Gateway, L3-SW, WiFi APs
