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
        ├── pvc.yaml         # comfyui-data (models) and comfyui-state
        ├── service.yaml
        └── httproute.yaml   # HTTPRoute → shared-gateway-envoy
```

## Access

Exposed via Gateway API HTTPRoute at `comfyui.prd.butaco.net`.

## GPU

Requests one `amd.com/gpu` on a `gpu=amd` labelled and tainted node.

The container uses unconfined seccomp. The device plugin injects permitted
`/dev/kfd` and `/dev/dri` nodes; never shadow them with hostPath mounts.

### ROCm

The custom image bakes PyTorch ROCm 10 wheels for gfx1200 and their userspace;
it does not use the host ROCm install. Do not set `HSA_OVERRIDE_GFX_VERSION`.

## Image Updates

The Forgejo repository `takanao/comfyui-docker`
pins `COMFYUI_VERSION`, which self-hosted Renovate on Forgejo bumps. Each build
pushes `<COMFYUI_VERSION>-<short sha>` and opens a `manual-review` pull request
here that sets `image.tag` in `chart/values.yaml`; merging it deploys, reverting
it rolls back. With `replicaCount: 0` the new tag takes effect the next time
gpu-switch starts ComfyUI. See [ADR-0054](../../docs/adr/0054-comfyui-image-updates-via-ci-opened-pull-requests.md).

## Storage

| PVC | Default Size | Mount Path |
|-----|-------------|------------|
| `comfyui-data` | 100Gi | `/app/ComfyUI/models` |
| `comfyui-state` | 20Gi | `input`, `output`, `user`, `custom_nodes` under `/app/ComfyUI` (subPaths) |

ComfyUI-Manager keeps its state under `user`. Custom nodes it installs persist,
but their pip dependencies live in the image venv and are lost when the pod is
recreated; bake nodes with extra dependencies into the image instead.

## Key Values

| Key | Default | Description |
|-----|---------|-------------|
| `hostname` | `comfyui.prd.butaco.net` | HTTPRoute hostname |
| `replicaCount` | `0` | Set to `1` to start (default off to save GPU) |
| `image.repository` | `forgejo.home.butaco.net/takanao/comfyui-docker` | Custom ROCm-enabled ComfyUI image |
| `image.tag` | set by comfyui-docker CI | Immutable `<COMFYUI_VERSION>-<short sha>` tag |
| `storage.size` | `100Gi` | PVC size for model storage |
| `storage.storageClassName` | `openebs-hostpath` | Storage class for both PVCs |
| `stateStorage.size` | `20Gi` | PVC size for inputs, outputs, user data, and custom nodes |

## Notes

- `replicaCount: 0` leaves GPU activation to gpu-switch; Argo CD ignores drift.
