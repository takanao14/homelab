# ADR-0029: ROCm serves concurrent MXFP4 workloads; Vulkan remains faster for one request

- **Status:** Superseded by [ADR-0039](0039-builtin-rocm-backend-replaces-custom-lemonade-image.md)
- **Date:** 2026-07-30
- **Related:** [ADR-0028](0028-lemonade-vulkan-backend-over-rocm-on-rdna4.md) (candidate to supersede in part),
  [ADR-0027](0027-gpu-workload-switching-web-ui.md),
  [`k8s/lemonade-server/README.md`](../../k8s/lemonade-server/README.md)

## Context

ADR-0028 chose Vulkan after measuring Qwen3-4B ~20% faster for one request and
~47% faster for four, while retaining the ROCm Deployment for re-measurement.

MXFP4 shows that the result depends on quantization and concurrency: Vulkan wins
one request, while ROCm wins four-request aggregate throughput.

A short Qwen3-4B re-measurement reproduced ADR-0028: 98.72 tok/s on Vulkan versus
82.08 on ROCm, with Vulkan about 40% faster at four requests.

## Measurement

Both Deployments ran Lemonade 10.8.0 on the same RX 9060 XT and shared model
PVCs. ROCm used the custom `b1302` image and `system` backend; Vulkan used the
upstream image and `vulkan` backend. After a warm-up request, each single-request
result is the median of five 256-token completions. Four-request aggregate
throughput is the median of five batches, with a three-second cooldown between
batches; each batch generated 1,024 completion tokens in total.

| Load | Vulkan | ROCm | Result |
|------|-------:|-----:|--------|
| One request, server generation rate | 98.17 tok/s | 88.29 tok/s | Vulkan 11.2% faster |
| One request, end-to-end completion throughput | 86.95 tok/s | 82.16 tok/s | Vulkan 5.8% faster |
| Four concurrent requests, aggregate end-to-end throughput | 97.13 tok/s | 117.65 tok/s | ROCm 21.1% faster |

The fixed request used 87 prompt tokens, `temperature: 0`, and
`max_tokens: 256`. The first four-request batch varied substantially on both
backends, so the decision uses medians rather than the best batch.

No non-MXFP4 client is configured in this repository. `opencode.json` is the
only OpenAI-compatible client configuration and selects
`gpt-oss-20b-mxfp4-GGUF`; the Vulkan Deployment remains available for
re-measurement and for single-request latency/throughput, not as the configured
primary for another model.

## Decision

**Keep this ADR Proposed: the measurement does not support a quantization-only
backend switch.** ROCm is the serving path when four-request aggregate
throughput is the objective; Vulkan is the faster path for a single request.
Before accepting this ADR, the intended client workload must explicitly
prioritize one of those two cases.

`opencode.json` selects `lemonade-rocm/gpt-oss-20b-mxfp4-GGUF`. Both Deployments
stay at `replicaCount: 0` and remain GPU-exclusive through `gpu-switch`
(ADR-0027). The current client therefore chooses concurrent aggregate throughput
at the cost of single-request generation speed.

Backend choice still lives in `config.json` on the recipe PVC, pinned per
Deployment by the `jq` initContainer, exactly as ADR-0028 established — so the two
Deployments can disagree about the backend without either corrupting the other.

## Consequences

- **ADR-0028's central claim is now scoped, not wrong.** Vulkan is faster on the
  Qwen3-4B model it measured and on a single MXFP4 request; it is not faster
  universally. Backend choice depends on model, quantization, and concurrency,
  so neither Deployment can be deleted.
- The Forgejo ROCm image now enters a serving path. Renovate cannot see its base
  tag or `LLAMACPP_ROCM_VERSION`, so updates require manual monitoring.
- **The bundled ROCm userspace/KMD pairing matters; the host ROCm userspace
  still does not.** The custom image supplies its own ROCm 7 libraries, so
  lemonade does not load `/opt/rocm` from [`ansible/roles/rocm`](../../ansible/roles/rocm/).
  Driver upgrades from that role must nevertheless re-check compatibility with
  the image, alongside comfyui and ollama.
- **The ROCm Deployment is no longer benchmark-only.** Its custom image is on
  the configured serving path even while this ADR remains Proposed, so its
  README and chart comments must describe both serving and benchmark roles.

## Alternatives considered

- **Keep Vulkan for MXFP4 unconditionally** — preserves ADR-0028 and avoids the
  custom serving image. It is the correct choice for one request, but leaves
  21.1% aggregate throughput on the table in the measured four-request case.
- **Use ROCm for MXFP4 unconditionally** — matches the current `opencode.json`
  endpoint and optimizes the measured concurrent case, but makes a single
  request 11.2% slower at generation. The measurement does not justify this as
  a universal rule.
- **Serve MXFP4 from a non-MXFP4 quantization on Vulkan** — sidesteps the backend
  question by changing the model instead. Not evaluated; worth measuring before
  this ADR moves to Accepted, since it would let the custom image go away
  entirely.
- **Fold this into ADR-0028 as an edit** — rejected by convention: `docs/adr/`
  is append-only and a changed decision gets a new record
  ([docs/adr/README.md](README.md)).
