# MeshCentral on Kubernetes

Helm chart for deploying MeshCentral on the homelab Kubernetes cluster, managed via Helmfile.

## Directory Structure

```
meshcentral/
├── helmfile.yaml
├── values.yaml.gotmpl       # Environment-specific values (via requiredEnv)
├── .envrc                   # Sets env vars (gitignored)
└── chart/                   # Helm chart
    ├── Chart.yaml
    ├── values.yaml
    └── templates/
        ├── deployment.yaml  # Includes checksum annotation for auto-restart on ConfigMap change
        ├── configmap.yaml
        ├── service.yaml
        └── pvc.yaml
```

## Prerequisites

- Kubernetes cluster
- CNI: Cilium with L2 LoadBalancer enabled
- A default StorageClass
- `helm`, `helmfile`, `kubectl`

## Deployment

### 1. Set up secrets

```bash
cd k8s/meshcentral
sops edit secrets.enc.env
direnv allow
```

### 2. Deploy

```bash
# Verify manifests
helmfile template

# Apply to cluster
helmfile apply
```

## Secret Variables

| Variable | Description |
|----------|-------------|
| `MESHCENTRAL_LB_IP` | Static IP address for the LoadBalancer Service |

The IP is injected via `values.yaml.gotmpl` into the `io.cilium/load-balancer-ip` annotation.

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
