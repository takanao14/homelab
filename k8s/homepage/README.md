# Homepage

[Homepage](https://gethomepage.dev/) dashboard deployed on the homelab Kubernetes cluster.

## Directory Structure

```
k8s/homepage/
├── values.yaml            # Environment values (hostname, Proxmox URLs, etc.)
├── secrets.enc.yaml       # SOPS-encrypted secrets (YAML format)
└── chart/                 # Custom Helm chart
    ├── Chart.yaml
    ├── values.yaml        # Default chart values
    ├── config/            # Homepage YAML configurations (tpl expanded)
    │   ├── settings.yaml
    │   ├── services.yaml
    │   ├── widgets.yaml
    │   └── ...
    └── templates/         # Kubernetes manifests
        ├── secret-config.yaml # Mounts config/*.yaml as Secrets
        ├── deployment.yaml
        ├── service.yaml
        └── rbac.yaml
```

## Deployment

This application is managed by **ArgoCD** with the `helm-secrets` plugin.

### 1. Set up secrets

Secrets are managed using SOPS.

```bash
cd k8s/homepage
sops edit secrets.enc.yaml
```

### 2. Apply via ArgoCD

The application is defined in `k8s/argocd/prd/apps/homepage.yaml`. Changes pushed to the `main` branch are automatically synchronized by ArgoCD.

To manually trigger a sync:
- Use the ArgoCD UI.
- Or use `argocd app sync homepage`.

## Configuration

Homepage configuration (`services.yaml`, `widgets.yaml`, `settings.yaml`, etc.) is located in the `chart/config/` directory.

- These files are standard Homepage YAMLs, but they can contain Helm templates (`{{ .Values... }}`).
- The `chart/templates/secret-config.yaml` template automatically reads all `*.yaml` files in `chart/config/` and creates a Kubernetes Secret named `homepage-config`.
- Credentials and environment-specific URLs are injected from `values.yaml` and `secrets.enc.yaml`.

## Secret Structure

The following structure is expected in `secrets.enc.yaml`:

| Path | Description |
|------|-------------|
| `proxmox.prd.password` | Proxmox VE password/token (production) |
| `proxmox.dev.password` | Proxmox VE password/token (development) |
| `truenas.key` | TrueNAS API key |
| `grafana.password` | Grafana admin password |

## Services Displayed

- **DNS**: PowerDNS auth1/auth2, dnsdist1/dnsdist2
- **VM/Storage**: Proxmox VE (Prd/Dev/Prd2), TrueNAS
- **Monitoring**: Grafana, Prometheus
- **Develop**: Forgejo
- **Network**: Border Gateway, L3-SW, WiFi APs
