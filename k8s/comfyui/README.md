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
        ├── deployment.yaml  # Recreate strategy; GPU devices come from the device plugin
        ├── pvc.yaml
        ├── service.yaml
        └── httproute.yaml   # HTTPRoute → shared-gateway-envoy
```

## Access

Exposed via Gateway API HTTPRoute at `comfyui.prd.butaco.net`.

> `butaco.net` is a personal domain. Replace it in `values.yaml`.

## GPU

Requires one AMD GPU (`amd.com/gpu: "1"`), allocated by the ROCm k8s-device-plugin deployed from `k0s/helmfile.yaml.gotmpl`. The node must be labeled `gpu: amd` and tainted `gpu=amd:NoSchedule` (applied automatically by the k0s cluster setup for GPU workers).

The container uses an unconfined seccomp profile. It does **not** hostPath-mount `/dev/kfd` or `/dev/dri`: the device plugin injects `/dev/kfd` plus the allocated `/dev/dri/card<N>` and `/dev/dri/renderD<N>` in response to the `amd.com/gpu` request, and adds the matching device-cgroup rules. Bind-mounting the whole host `/dev/dri` on top would expose every card on the host while the cgroup still permits only the allocated one.

### ROCm

Unlike ollama and lemonade-server (which download or bundle their own ROCm userspace), ComfyUI runs on **PyTorch ROCm wheels baked into the custom image** at build time. The [`comfyui-docker` Dockerfile](https://forgejo.home.butaco.net/takanao/comfyui-docker) installs PyTorch from the `rocm7.2` wheel index (`--index-url https://download.pytorch.org/whl/rocm7.2`); those wheels bundle their own ROCm runtime, so the container is **independent of the host ROCm version** (`ansible/roles/rocm`, currently ROCm 7.14). Only the host amdgpu kernel driver has to stay within AMD's [KMD/UMD skew window](https://rocm.docs.amd.com/projects/install-on-linux/en/latest/reference/user-kernel-space-compat-matrix.html) (one year since ROCm 6.4), which ROCm 7.2 wheels satisfy against the 7.14 host driver.

PyTorch's `rocm7.2` wheels officially support `gfx1200`/`gfx1201` (RDNA4), so no `HSA_OVERRIDE_GFX_VERSION` is needed for the RX 9060 XT. The image was last built against `rocm7.2`; `rocm7.3`+ wheel indexes are not published yet, so this is the newest available and no bump is due for the ROCm 7.14 host upgrade. Rebuild the image only to move the PyTorch/ROCm wheel line — edit the `--index-url` in the Dockerfile, and the Forgejo Actions workflow rebuilds and pushes `:latest`.

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
