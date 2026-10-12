# Homepage

[Homepage](https://gethomepage.dev/) dashboard for prd, managed by Argo CD.

## Directory Structure

```
homepage/
├── prd/values.yaml        # prd overrides (Gateway https listener)
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

An HTTPRoute uses the environment hostname and listener.

## Configuration

`chart/config/` is mounted as native Homepage YAML. Keep Helm and environment
differences in values files, not these configs.

`secret-config.yaml` packages the config files as `homepage-config`.

## Secrets

ESO injects OpenBao values as `HOMEPAGE_VAR_*` variables used by Homepage.

OpenBao KV paths:

| OpenBao path | Property | Description |
|-------------|----------|-------------|
| `k8s/homepage/proxmox` | `{host}-token` | API token ID (`pve`, `node1`–`node5`; `user@pam!tokenname`) |
| `k8s/homepage/proxmox` | `{host}-secret` | Matching API token secret |
| `k8s/homepage/truenas` | `key` | TrueNAS API key |
| `k8s/homepage/grafana` | `password` | Grafana admin password |
