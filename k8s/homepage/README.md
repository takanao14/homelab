# Homepage

[Homepage](https://gethomepage.dev/) dashboard deployed on the prd and sandbox clusters. Managed by ArgoCD.

## Directory Structure

```
homepage/
└── chart/                 # Custom Helm chart
    ├── Chart.yaml
    ├── values.yaml        # Kubernetes deployment values (hostname, image, Gateway)
    ├── config/            # Homepage YAML configs mounted as-is
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
        ├── httproute.yaml      # HTTPRoute → shared-gateway-envoy
        └── rbac.yaml
```

## Access

Exposed via Gateway API HTTPRoute. Hostname is set in `chart/values.yaml` and
overridden per environment by the ArgoCD Application. Sandbox uses HTTP via the
`http` Gateway listener at `http://homepage.sandbox.butaco.net`.

> `butaco.net` is a personal domain. Replace it in `chart/values.yaml`.

## Configuration

Homepage configs (`services.yaml`, `widgets.yaml`, etc.) are in `chart/config/`.
They are mounted as-is and should stay close to Homepage's native YAML format.
Do not use Helm templating in these files; keep deployment differences in
`values.yaml` or in the per-environment ArgoCD Application.

`secret-config.yaml` reads all `*.yaml` files in `chart/config/` and creates a Kubernetes Secret named `homepage-config`.

## Secrets

All secrets are fetched from OpenBao via ESO. They are injected as environment variables (`HOMEPAGE_VAR_*`) that Homepage substitutes in its config files.

Sandbox is used for staging validation and intentionally reuses the same
dashboard Secret paths as prd. The `kubernetes-sandbox` OpenBao auth role must
include the `k8s-homepage` policy so ESO can read `k8s/homepage/*`.

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
