# Homepage

[Homepage](https://gethomepage.dev/) dashboard for prd and sandbox, managed by Argo CD.

## Directory Structure

```
homepage/
├── prd/values.yaml        # prd overrides (Gateway https listener)
├── sandbox/values.yaml    # sandbox overrides (hostname, http listener)
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

An HTTPRoute uses the environment hostname and listener; sandbox is HTTP-only.

> `butaco.net` is a personal domain. Replace it in `chart/values.yaml`.

## Configuration

`chart/config/` is mounted as native Homepage YAML. Keep Helm and environment
differences in values files, not these configs.

`secret-config.yaml` packages the config files as `homepage-config`.

## Secrets

ESO injects OpenBao values as `HOMEPAGE_VAR_*` variables used by Homepage.

Sandbox intentionally reuses prd dashboard paths; its OpenBao role therefore
requires the `k8s-homepage` policy.

OpenBao KV paths:

| OpenBao path | Property | Description |
|-------------|----------|-------------|
| `k8s/homepage/proxmox` | `pve-token` | Proxmox VE API token ID (pve, format: `user@pam!tokenname`) |
| `k8s/homepage/proxmox` | `pve-secret` | Proxmox VE API token secret (pve) |
| `k8s/homepage/proxmox` | `node1-token` | Proxmox VE API token ID (node1) |
| `k8s/homepage/proxmox` | `node1-secret` | Proxmox VE API token secret (node1) |
| `k8s/homepage/proxmox` | `node2-token` | Proxmox VE API token ID (node2) |
| `k8s/homepage/proxmox` | `node2-secret` | Proxmox VE API token secret (node2) |
| `k8s/homepage/proxmox` | `node3-token` | Proxmox VE API token ID (node3) |
| `k8s/homepage/proxmox` | `node3-secret` | Proxmox VE API token secret (node3) |
| `k8s/homepage/proxmox` | `node4-token` | Proxmox VE API token ID (node4) |
| `k8s/homepage/proxmox` | `node4-secret` | Proxmox VE API token secret (node4) |
| `k8s/homepage/proxmox` | `node5-token` | Proxmox VE API token ID (node5) |
| `k8s/homepage/proxmox` | `node5-secret` | Proxmox VE API token secret (node5) |
| `k8s/homepage/truenas` | `key` | TrueNAS API key |
| `k8s/homepage/grafana` | `password` | Grafana admin password |
