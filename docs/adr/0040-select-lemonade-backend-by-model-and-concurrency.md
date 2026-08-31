# ADR-0040: Select the Lemonade backend by model and concurrency

- **Status:** Accepted
- **Date:** 2026-09-01
- **Related:** [ADR-0029](0029-rocm-serving-path-for-mxfp4-models.md),
  [ADR-0039](0039-builtin-rocm-backend-replaces-custom-lemonade-image.md),
  [ADR-0027](0027-gpu-workload-switching-web-ui.md),
  [`k8s/lemonade-server/README.md`](../../k8s/lemonade-server/README.md)

## Context

ADR-0029 found that Vulkan was faster for one MXFP4 request while ROCm had
higher aggregate throughput at four concurrent requests. Those measurements
used Lemonade 10.8.0, separate Deployments, and a custom `b1302` image exposing
ROCm through the `system` backend. ADR-0039 retired that configuration in favor
of Lemonade 11.8.1 and per-model backend selection in one upstream container.

The old numbers therefore remain historical evidence but cannot characterize
the deployed stack. Phase 5 re-measured the same downloaded
`gpt-oss-20b-mxfp4-GGUF` model with the image, Lemonade process, checkpoint,
context size, and hardware held constant.

## Measurement

The 2026-09-01 test ran on the RX 9060 XT attached to `gpuvm1` with:

- Lemonade 11.8.1 image digest
  `sha256:203730bc4d7f3b53d4d71cbf25f62b0574552b8ec3bad3f289f3a9fcb79abf3d`
- `ggml-org/gpt-oss-20b-GGUF` snapshot
  `c9a07b972b8979719fd97b8ff2c10dc79ebf5d26`, MXFP4 artifact
- built-in ROCm `b1319` and Vulkan `b10375` backends
- context size 4096, 104 input tokens, and 256 completion tokens
- the stock `chat-long-output` scenario from `bench_scenarios.json`

The single-request test used `lemonade bench`, one warm-up, and five measured
runs per backend. The table reports medians. End-to-end throughput divides 256
completion tokens by the median request duration.

| Single-request metric | Vulkan | ROCm | Result |
|-----------------------|-------:|-----:|--------|
| Server generation rate | 98.14 tok/s | 85.73 tok/s | Vulkan 14.5% faster |
| End-to-end completion throughput | 85.92 tok/s | 52.75 tok/s | Vulkan 62.9% faster |
| Time to first token | 361 ms | 1,870 ms | Vulkan 80.7% lower |
| Peak VRAM | 10.96 GiB | 11.25 GiB | Vulkan 0.29 GiB lower |

The concurrency test loaded each backend with `--parallel 4`, issued the same
request four times concurrently, and set `temperature: 0` and
`max_tokens: 256`. After one warm-up, five batches ran with a three-second
cooldown. Every request returned 104 prompt and 256 completion tokens, so each
batch produced exactly 1,024 completion tokens.

| Backend | Aggregate throughput samples | Median |
|---------|------------------------------|-------:|
| Vulkan | 150.34, 164.54, 168.98, 167.16, 165.85 tok/s | 165.85 tok/s |
| ROCm | 184.12, 205.48, 190.55, 193.03, 193.98 tok/s | 193.03 tok/s |

ROCm was 16.4% faster at four-request aggregate throughput. The first batch was
slower on both backends, so the decision uses all five samples and their median,
not the best result.

This is a current-stack backend comparison, not a before/after comparison with
ADR-0029. Its original prompt body was not retained, and that test reported 87
prompt tokens rather than the 104-token stock v11 scenario.

## Decision

**Select the llama.cpp backend per model and expected concurrency; do not set a
universal Lemonade backend.** For this MXFP4 checkpoint, prefer Vulkan for a
single interactive request and ROCm when four-request aggregate throughput is
the objective.

Express the choice with the model's `recipe_options.llamacpp_backend` in the
single Lemonade Deployment. Do not recreate backend-specific Deployments or a
custom image. A client or model registration that needs a persistent choice
must state its workload objective; in the absence of such a client, these
results remain characterization rather than a cluster-wide default.

## Consequences

- ADR-0029's qualitative finding is reproduced on the v11 architecture, while
  its numeric results remain historical and are not reused as a baseline.
- Backend choice remains narrow to the measured model, quantization, hardware,
  context, and concurrency. It does not imply that ROCm or Vulkan wins for all
  models.
- Changes to Lemonade, either backend build, the model artifact, GPU driver, or
  the target concurrency require re-measurement before changing a persistent
  client choice.
- Both backend binaries remain in the recipe cache. This adds storage but not a
  second workload, Service, route, or custom-image maintenance path.

## Alternatives considered

- **Infer the decision from ADR-0029** — rejected because its image, Lemonade
  version, ROCm build, and backend integration differ from the deployed stack.
- **Choose Vulkan globally from the single-request result** — rejected because
  it gives up 16.4% aggregate throughput in the measured four-request case.
- **Choose ROCm globally from the concurrent result** — rejected because it is
  slower for the measured interactive case, especially before the first token.
- **Restore two Deployments for clean separation** — rejected because per-model
  recipe options already isolate the choice without duplicating infrastructure.
