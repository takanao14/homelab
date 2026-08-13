# vLLM

[vLLM](https://docs.vllm.ai/) OpenAI-compatible ROCm inference on prd.

## Access

| Client | Address |
|---|---|
| External | `https://vllm.prd.butaco.net` |
| In-cluster | `http://vllm.vllm.svc.cluster.local:8000` |

## GPU / ROCm

Requests one `amd.com/gpu` on the labelled and tainted GPU node.

Use AMD's Radeon/Navi gfx1200 image, not the Instinct/CDNA variant. Do not set
`HSA_OVERRIDE_GFX_VERSION`.

Service Links stay disabled because Kubernetes otherwise injects a URI into
vLLM's numeric `VLLM_PORT`.

Standard Qwen3 uses the Hermes parser for `tool_choice: auto`; revisit it when
changing model families.

## Validation

Zero replicas lets Argo CD create resources without consuming the GPU.

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

The unquantized `Qwen/Qwen3-4B` baseline validates the stack within 16 GiB VRAM;
test larger models only after it succeeds.

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
