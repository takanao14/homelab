# MeshCentral on Kubernetes

Kustomize manifests for deploying MeshCentral on Kubernetes (k0s).

## Directory Structure

*   `base/`: Common Deployment and Service (ClusterIP) definitions
*   `overlays/dev/`: Configuration for development environment (Namespace, PVC, LoadBalancer, ConfigMap)

## Prerequisites

*   **Kubernetes Cluster**
*   **CNI**: Cilium (L2 LoadBalancer feature must be enabled)
*   **Storage**: A default StorageClass must exist
*   **Tools**: Kustomize, kubectl

## Deployment

### 1. Configure Environment Variables

Create a `.env` file in the `overlays/dev` directory and set the required environment variables.
Specifically, `HOSTNAME` is used as the LoadBalancer static IP address.

```bash
# Example of creating overlays/dev/.env
echo "HOSTNAME=192.168.20.200" > overlays/dev/.env
```

**Required Variables in .env:**

*   `HOSTNAME`: LoadBalancer IP address for the MeshCentral service (e.g., `192.168.20.200`)

### 2. Deploy

```bash
# Verify manifests
kustomize build overlays/dev

# Apply to cluster
kustomize build overlays/dev | kubectl apply -f -
```

## Configuration Details

### Static IP Address (LoadBalancer)

The `replacements` feature in `overlays/dev/kustomization.yaml` automatically injects the value of the `HOSTNAME` variable from the `.env` file into the Service's `io.cilium/load-balancer-ip` annotation.
This allows you to manage the IP address centrally via environment variables.
The `HOSTNAME` environment variable should be set to the IP address or DNS name where MeshCentral will operate. Since it's configured as an ExternalIP here, an IP address is used.

### Storage (PVC)

The following PersistentVolumeClaims are defined in `base/pvc.yaml`.
The StorageClass is not explicitly set in the manifests, assuming the default StorageClass or set via overlays.

| PVC Name | Size | Description |
| :--- | :--- | :--- |
| `meshcentral-data` | 1Gi | Configuration data |
| `meshcentral-files` | 10Gi | Uploaded files |
| `meshcentral-backups` | 5Gi | Backups |
| `meshcentral-web` | 1Gi | Web content |
