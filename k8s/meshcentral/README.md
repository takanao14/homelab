# MeshCentral on Kubernetes

Helm chart for deploying MeshCentral on Kubernetes (k0s), managed via Helmfile.

## Directory Structure

*   `chart/`: Helm chart for MeshCentral
*   `helmfile.yaml`: Helmfile release definition
*   `values.yaml.gotmpl`: Environment-specific values (injected via environment variables)
*   `.envrc.sample`: Sample environment variable definitions

## Prerequisites

*   **Kubernetes Cluster**
*   **CNI**: Cilium (L2 LoadBalancer feature must be enabled)
*   **Storage**: A default StorageClass must exist
*   **Tools**: Helm, Helmfile, kubectl

## Deployment

### 1. Configure Environment Variables

```bash
cp .envrc.sample .envrc
# Edit .envrc with actual values
```

| Variable | Description |
| :--- | :--- |
| `MESHCENTRAL_LB_IP` | Static IP address for the LoadBalancer |

### 2. Load Environment Variables

```bash
# Using direnv
direnv allow

# Or manually
source .envrc
```

### 3. Deploy

```bash
# Verify manifests
helmfile template

# Apply to cluster
helmfile apply
```

## Configuration Details

### Static IP Address (LoadBalancer)

`MESHCENTRAL_LB_IP` is injected via `values.yaml.gotmpl` into the Service's `io.cilium/load-balancer-ip` annotation, allowing Cilium to assign a static IP to the LoadBalancer.

### Storage (PVC)

PersistentVolumeClaims are defined in `chart/templates/pvc.yaml` and sized via `chart/values.yaml`.

| PVC Name | Default Size | Mount Path |
| :--- | :--- | :--- |
| `meshcentral-data` | 1Gi | `/opt/meshcentral/meshcentral-data` |
| `meshcentral-files` | 10Gi | `/opt/meshcentral/meshcentral-files` |
| `meshcentral-backups` | 5Gi | `/opt/meshcentral/meshcentral-backups` |
| `meshcentral-web` | 1Gi | `/opt/meshcentral/meshcentral-web` |

### Resource Limits

Default resource requests/limits defined in `chart/values.yaml`:

| | CPU | Memory |
| :--- | :--- | :--- |
| Requests | 100m | 256Mi |
| Limits | 1000m | 1Gi |
