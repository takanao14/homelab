# vLLM

[vLLM](https://docs.vllm.ai/) OpenAI-compatible inference server deployed on
the prd cluster with AMD GPU (ROCm) support. Managed by Argo CD.

## Access

| Client | Address |
|---|---|
| External | `https://vllm.prd.butaco.net` |
| In-cluster | `http://vllm.vllm.svc.cluster.local:8000` |

## GPU / ROCm

The workload requests one `amd.com/gpu` resource and is pinned to the
`gpu=amd` node. The node must also carry the `gpu=amd:NoSchedule` taint.

The image is AMD's Radeon/Navi vLLM build. The `_navi_` image variant is
required for the RX 9060 XT (`gfx1200`); the CDNA variant targets AMD Instinct
GPUs. Do not set `HSA_OVERRIDE_GFX_VERSION` because the GPU target is supported
natively.

The pod disables Kubernetes Service Links. Without
`enableServiceLinks: false`, the `vllm` Service injects a
`VLLM_PORT=tcp://<service-ip>:8000` environment variable. vLLM reserves that
name for a numeric internal port and exits when Kubernetes supplies the URI.

Automatic tool choice is enabled with the `hermes` parser for the standard
Qwen3 model. This is required when Open WebUI sends `tool_choice: auto`.
Revisit the parser when changing model families; Qwen3-Coder, for example,
uses a different parser.

## Validation

The deployment defaults to zero replicas so Argo CD can create the namespace,
route, service, and model-cache PVC without consuming the single GPU.

Start it through the exclusive GPU workload switch:

```bash
scripts/gpu-switch.sh vllm
kubectl -n vllm rollout status deploy/vllm --timeout=20m
kubectl -n vllm logs deploy/vllm
curl -fsS https://vllm.prd.butaco.net/health
curl -fsS https://vllm.prd.butaco.net/v1/models
```

Verify the pod is using the expected node and GPU allocation:

```bash
kubectl -n vllm get pod -o wide
kubectl -n vllm describe pod -l app=vllm
```

The initial model is `Qwen/Qwen3-4B`. This deliberately avoids quantization
while validating the ROCm, PyTorch, and vLLM stack within the GPU's 16 GiB
VRAM. Test larger or quantized models only after this baseline succeeds.

## Key values

| Key | Default | Description |
|---|---|---|
| `replicaCount` | `0` | Started only through `gpu-switch.sh` |
| `model.id` | `Qwen/Qwen3-4B` | Hugging Face model ID |
| `model.maxModelLen` | `32768` | Maximum context length |
| `model.gpuMemoryUtilization` | `0.90` | Fraction of VRAM vLLM may reserve |
| `toolCalling.enabled` | `false` (chart) / `true` (app values) | Accept `tool_choice: auto` |
| `toolCalling.parser` | unset (chart) / `hermes` (app values) | Model-specific tool-call parser |
| `storage.size` | `100Gi` | Hugging Face cache PVC |
| `sharedMemory.sizeLimit` | `8Gi` | Memory-backed `/dev/shm` |
