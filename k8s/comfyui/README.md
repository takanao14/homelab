# comfyui

[ComfyUI](https://github.com/comfyanonymous/ComfyUI) image generation on prd's AMD GPU.

## Directory Structure

```
comfyui/
├── values.yaml           # Environment-level overrides (hostname, replicaCount)
└── chart/                # Custom Helm chart
    ├── Chart.yaml
    ├── values.yaml       # Default chart values
    └── templates/
        ├── deployment.yaml  # Recreate strategy; GPU devices come from the device plugin
        ├── pvc.yaml
        ├── service.yaml
        └── httproute.yaml   # HTTPRoute → shared-gateway-envoy
```

## Access

Exposed via Gateway API HTTPRoute at `comfyui.prd.butaco.net`.

> `butaco.net` is a personal domain. Replace it in `values.yaml`.

## GPU

Requests one `amd.com/gpu` on a `gpu=amd` labelled and tainted node.

The container uses unconfined seccomp. The device plugin injects permitted
`/dev/kfd` and `/dev/dri` nodes; never shadow them with hostPath mounts.

### ROCm

The custom image bakes PyTorch `rocm7.2` wheels and their userspace, independent
of host ROCm 7.14 except for AMD's supported driver/userspace skew window.

`rocm7.2` supports gfx1200 natively; do not set `HSA_OVERRIDE_GFX_VERSION`.
Rebuild only when changing the PyTorch/ROCm wheel line.

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

- `replicaCount: 0` leaves GPU activation to gpu-switch; Argo CD ignores drift.
- Forgejo hosts the custom ROCm image.
