# MeshCentral

[MeshCentral](https://meshcentral.com/) remote management server deployed on the dev cluster. Managed by ArgoCD.

## Directory Structure

```
meshcentral/
├── values.yaml            # hostname: meshcentral.dev.butaco.net
└── chart/                 # Custom Helm chart
    ├── Chart.yaml
    ├── values.yaml        # Default chart values
    └── templates/
        ├── deployment.yaml  # Checksum annotation for auto-restart on ConfigMap change
        ├── configmap.yaml
        ├── service.yaml     # ClusterIP
        └── pvc.yaml
```

## Access

Exposed via Gateway API HTTPRoute. Hostname is set in `values.yaml`.

> `butaco.net` is a personal domain. Replace it in `values.yaml`.

## Storage

| PVC Name | Default Size | Mount Path |
|----------|-------------|-----------|
| `meshcentral-data` | 1Gi | `/opt/meshcentral/meshcentral-data` |
| `meshcentral-files` | 10Gi | `/opt/meshcentral/meshcentral-files` |
| `meshcentral-backups` | 5Gi | `/opt/meshcentral/meshcentral-backups` |
| `meshcentral-web` | 1Gi | `/opt/meshcentral/meshcentral-web` |

## Resource Limits

| | CPU | Memory |
|-|-----|--------|
| Requests | 100m | 256Mi |
| Limits | 1000m | 1Gi |
