# lemonade-server

[Lemonade](https://github.com/lemonade-sdk/lemonade) LLM inference server deployed on the dev cluster with AMD GPU (ROCm) support. Managed by ArgoCD.

## Directory Structure

```
lemonade-server/
├── values.yaml           # Environment-level overrides (hostname)
└── chart/                # Custom Helm chart
    ├── Chart.yaml
    ├── values.yaml       # Default chart values
    └── templates/
        ├── deployment.yaml  # Recreate strategy; AMD GPU resource requests
        ├── pvc.yaml         # HuggingFace cache, llama.cpp binaries, recipe cache
        ├── service.yaml     # ClusterIP on port 13305
        └── httproute.yaml   # HTTPRoute → shared-gateway
```

## Access

| Client | Address |
|--------|---------|
| External | `https://lemonade.dev.butaco.net` |
| In-cluster | `http://lemonade-server.lemonade-server.svc.cluster.local:13305` |

## GPU

Requires one AMD GPU (`amd.com/gpu: "1"`). The node must be labeled `gpu: amd` and tainted `gpu=amd:NoSchedule`.

Uses `ghcr.io/lemonade-sdk/lemonade-server` with `LEMONADE_LLAMACPP=rocm` to enable ROCm acceleration.

## Storage

| PVC | Default Size | Mount Path |
|-----|-------------|------------|
| `lemonade-huggingface` | 90Gi | `/root/.cache/huggingface` |
| `lemonade-llama` | 5Gi | `/opt/lemonade/llama` |
| `lemonade-recipe` | 5Gi | `/root/.cache/lemonade` |

## Key Values

| Key | Default | Description |
|-----|---------|-------------|
| `hostname` | `lemonade.dev.butaco.net` | HTTPRoute hostname |
| `replicaCount` | `0` | Set to `1` to start (default off to save GPU) |
| `image.tag` | `v10.4.0` | Lemonade server image tag |
| `storage.storageClassName` | `openebs-hostpath` | Storage class |
