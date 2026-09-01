# lemonade-server

[Lemonade](https://github.com/lemonade-sdk/lemonade) v11 inference on the prd
AMD GPU worker. One upstream Deployment can select Vulkan or ROCm per model;
the built-in ROCm backend is the validated serving path.

## Directory Structure

```
lemonade-server/
├── README.md
├── values.yaml           # Common Argo CD overrides
├── prd/
│   └── values.yaml       # prd-specific overrides
└── chart/
    ├── Chart.yaml
    ├── values.yaml       # Chart defaults and Lemonade sparse config
    └── templates/
        ├── configmap.yaml
        ├── deployment.yaml
        ├── httproute.yaml
        ├── pvc.yaml
        └── service.yaml
```

## Access

| Client | Address |
|--------|---------|
| External | `https://lemonade.prd.butaco.net` |
| In-cluster | `http://lemonade-server.lemonade-server.svc.cluster.local:13305` |

The route currently has no Lemonade API key. Keep its Gateway exposure within
the trusted network; configure `LEMONADE_API_KEY` from a Secret before exposing
it more broadly. Lemonade v11 otherwise exposes inference and `/internal/*`
control endpoints without authentication.

Lemonade accepts browser requests only from loopback origins (`127.0.0.1`,
`[::1]`, `*.localhost`) unless `LEMONADE_ALLOWED_ORIGINS` names more. The
Deployment therefore sets it to `https://{{ hostname }}`.

## Runtime Shape

The chart runs `ghcr.io/lemonade-sdk/lemonade-server:v11.8.1` as uid 10001 and
gid 999. It starts `/opt/lemonade/lemond --host 0.0.0.0` and requests one
`amd.com/gpu`.

The sparse `config.json` is generated from `config` in `chart/values.yaml` and
copied from a ConfigMap into a writable `emptyDir` on every Pod start. This
keeps the release configuration declarative while allowing `lemond` to update
the file during the Pod lifetime. Runtime config changes made through the API
do not survive a Pod restart.

The release pins:

```yaml
config:
  rocm_channel: nightly
  llamacpp:
    rocm_bin: b1319
```

`b1319` is a release tag from `lemonade-sdk/llamacpp-rocm`. Do not use
`latest`; update the tag deliberately and re-run the offload verification.
Backend choice is per model in v11, for example `--llamacpp rocm` or
`recipe_options.llamacpp_backend=rocm`.

See [ADR-0039](../../docs/adr/0039-builtin-rocm-backend-replaces-custom-lemonade-image.md)
for the migration from the v10 custom image and dual-Deployment layout.
[ADR-0040](../../docs/adr/0040-select-lemonade-backend-by-model-and-concurrency.md)
records the v11 backend comparison and requires choosing Vulkan or ROCm per
model and expected concurrency rather than setting a universal default.

## GPU Switching

The Deployment defaults to zero replicas and is managed through gpu-switch:

```bash
scripts/gpu-switch.sh lemonade-server
scripts/gpu-switch.sh status
```

Switch to another workload, or stop all GPU workloads, when finished:

```bash
scripts/gpu-switch.sh ollama
# scripts/gpu-switch.sh off
```

## Installing and Verifying ROCm

Check the configured backend and install the pinned binary when the recipe PVC
does not contain it yet:

```bash
kubectl -n lemonade-server exec deploy/lemonade-server -- \
  /opt/lemonade/lemonade config | grep -E 'rocm_channel|llamacpp.rocm_bin'
kubectl -n lemonade-server exec deploy/lemonade-server -- \
  /opt/lemonade/lemonade backends --all
kubectl -n lemonade-server exec deploy/lemonade-server -- \
  /opt/lemonade/lemonade backends install llamacpp:rocm
```

Load an existing model with ROCm explicitly and run a short request:

```bash
kubectl -n lemonade-server exec deploy/lemonade-server -- \
  /opt/lemonade/lemonade load Qwen3-0.6B-GGUF --llamacpp rocm --ctx-size 2048
kubectl -n lemonade-server exec deploy/lemonade-server -- \
  curl --fail-with-body -sS http://127.0.0.1:13305/api/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{"model":"Qwen3-0.6B-GGUF","messages":[{"role":"user","content":"Reply with OK."}],"max_tokens":32,"temperature":0,"stream":false}'
```

Confirm all three signals:

1. `lemonade status` reports the model as `gpu`, `ready`.
2. Logs contain `Using LlamaCpp Backend: rocm-nightly` and a successful request.
3. Prometheus `amd_gpu_used_vram` rises after the load.

The 2026-09-01 migration validation loaded Qwen3-0.6B Q4_0 at context 2048,
completed an OpenAI-compatible request, and moved VRAM from 65 MiB to 901 MiB.
The running backend reported `b1319`.

## Storage

| PVC | Default Size | Mount Path |
|-----|-------------:|------------|
| `lemonade-huggingface` | 90Gi | `/opt/lemonade/.cache/huggingface` |
| `lemonade-recipe` | 5Gi | `/opt/lemonade/.cache/lemonade` |

The Pod uses `fsGroup: 999` with `fsGroupChangePolicy: OnRootMismatch`. The
existing caches were migrated to gid 999 before the v11 rollout. The old empty
`lemonade-llama` PVC was removed because mounting it at `/opt/lemonade/llama`
would hide files shipped in the v11 image.

## Configuration Precedence

Argo CD applies values in this order, with later files taking precedence:

1. [`chart/values.yaml`](chart/values.yaml) — chart defaults
2. [`values.yaml`](values.yaml) — common overrides
3. [`prd/values.yaml`](prd/values.yaml) — prd overrides
