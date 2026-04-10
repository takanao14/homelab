# Homepage

[Homepage](https://gethomepage.dev/) dashboard deployed on the prd cluster. Managed by ArgoCD with the helm-secrets plugin.

## Directory Structure

```
homepage/
├── secrets.enc.yaml       # SOPS-encrypted credentials
└── chart/                 # Custom Helm chart
    ├── Chart.yaml
    ├── values.yaml        # hostname, Proxmox URLs, internal host addresses
    ├── config/            # Homepage YAML configs (Helm template expanded)
    │   ├── settings.yaml
    │   ├── services.yaml
    │   ├── widgets.yaml
    │   ├── bookmarks.yaml
    │   ├── kubernetes.yaml
    │   └── proxmox.yaml
    └── templates/
        ├── secret-config.yaml  # Mounts config/*.yaml as a Secret
        ├── deployment.yaml
        ├── service.yaml        # ClusterIP
        ├── httproute.yaml      # HTTPRoute → shared-gateway
        └── rbac.yaml
```

## Access

Exposed via Gateway API HTTPRoute. Hostname is set in `chart/values.yaml`.

> `butaco.net` is a personal domain. Replace it in `chart/values.yaml`.

## Configuration

Homepage configs (`services.yaml`, `widgets.yaml`, etc.) are in `chart/config/`. These are standard Homepage YAMLs that also support Helm template syntax (`{{ .Values... }}`).

`secret-config.yaml` reads all `*.yaml` files in `chart/config/` and creates a Kubernetes Secret named `homepage-config`.

## Secrets

```bash
sops edit k8s/homepage/secrets.enc.yaml
```

| Path | Description |
|------|-------------|
| `proxmox.prd.token` | Proxmox VE API token ID (prd, format: `user@pam!tokenname`) |
| `proxmox.prd.secret` | Proxmox VE API token secret (prd) |
| `proxmox.dev.token` | Proxmox VE API token ID (dev) |
| `proxmox.dev.secret` | Proxmox VE API token secret (dev) |
| `truenas.key` | TrueNAS API key |
| `grafana.password` | Grafana admin password |
