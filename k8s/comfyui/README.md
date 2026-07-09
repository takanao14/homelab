# comfyui

[ComfyUI](https://github.com/comfyanonymous/ComfyUI) AI image generation deployed on the prd cluster with AMD GPU support. Managed by ArgoCD.

## Directory Structure

```
comfyui/
├── values.yaml           # Environment-level overrides (hostname, replicaCount)
└── chart/                # Custom Helm chart
    ├── Chart.yaml
    ├── values.yaml       # Default chart values
    └── templates/
        ├── deployment.yaml  # Recreate strategy; mounts /dev/kfd and /dev/dri for ROCm
        ├── pvc.yaml
        ├── service.yaml
        └── httproute.yaml   # HTTPRoute → shared-gateway-envoy
```

## Access

Exposed via Gateway API HTTPRoute at `comfyui.prd.butaco.net`.

> `butaco.net` is a personal domain. Replace it in `values.yaml`.

## GPU

Requires one AMD GPU (`amd.com/gpu: "1"`). The node must be labeled `gpu: amd` and tainted `gpu=amd:NoSchedule` (applied automatically by the k0s cluster setup for GPU workers).

The container uses an unconfined seccomp profile and mounts `/dev/kfd` and `/dev/dri` as hostPath volumes for ROCm access.

## Storage

| PVC | Default Size | Mount Path |
|-----|-------------|------------|
| `comfyui-data` | 100Gi | `/app/ComfyUI/models` |

## Key Values

| Key | Default | Description |
|-----|---------|-------------|
| `hostname` | `comfyui.prd.butaco.net` | HTTPRoute hostname |
| `replicaCount` | `0` | Set to `1` to start (default off to save GPU) |
| `image.repository` | `forgejo.home.butaco.net/takanao/comfyui-docker` | Custom ROCm-enabled ComfyUI image |
| `storage.size` | `100Gi` | PVC size for model storage |
| `storage.storageClassName` | `openebs-hostpath` | Storage class |

## Notes

- `replicaCount` defaults to `0` (scaled down when not in use). ArgoCD ignores replica drift via `ignoreDifferences`.
- Uses a custom Docker image (`comfyui-docker`) built with ROCm support, hosted on the self-hosted Forgejo instance.
