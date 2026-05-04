# Homepage

[Homepage](https://gethomepage.dev/) dashboard deployed on the prd cluster. Managed by ArgoCD with the helm-secrets plugin.

## Directory Structure

```
homepage/
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
        ├── external-secret.yaml # ESO ExternalSecret for API credentials
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

All secrets are fetched from OpenBao via ESO. They are injected as environment variables (`HOMEPAGE_VAR_*`) that Homepage substitutes in its config files.

OpenBao KV paths:

| OpenBao path | Property | Description |
|-------------|----------|-------------|
| `k8s/homepage/proxmox` | `prd-token` | Proxmox VE API token ID (prd, format: `user@pam!tokenname`) |
| `k8s/homepage/proxmox` | `prd-secret` | Proxmox VE API token secret (prd) |
| `k8s/homepage/proxmox` | `dev-token` | Proxmox VE API token ID (dev) |
| `k8s/homepage/proxmox` | `dev-secret` | Proxmox VE API token secret (dev) |
| `k8s/homepage/proxmox` | `node2-token` | Proxmox VE API token ID (node2) |
| `k8s/homepage/proxmox` | `node2-secret` | Proxmox VE API token secret (node2) |
| `k8s/homepage/proxmox` | `node3-token` | Proxmox VE API token ID (node3) |
| `k8s/homepage/proxmox` | `node3-secret` | Proxmox VE API token secret (node3) |
| `k8s/homepage/truenas` | `key` | TrueNAS API key |
| `k8s/homepage/grafana` | `password` | Grafana admin password |
