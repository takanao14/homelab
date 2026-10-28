# ADR-0080: Run Strata as a gpu-switch workload on a 48 GiB GPU worker

- **Status:** Accepted
- **Category:** GPU inference
- **Date:** 2026-10-28
- **Related:** [ADR-0027](0027-gpu-workload-switching-web-ui.md),
  [ADR-0060](0060-mcp-relies-on-bearer-authentication.md),
  [`k8s/strata/README.md`](../../k8s/strata/README.md)

## Context

[Strata](https://github.com/Niko1221/Strata) serves Qwen3.8-Flash-Next MoE
packs by keeping the dense layers and part of the experts on the GPU and the
remaining experts in RAM. It supports only its own model catalog, not arbitrary
GGUF files. Upstream publishes a ROCm image for gfx120x, which covers the prd
RX 9060 XT (16 GB).

Strata's fit rule keeps 10 GB of RAM for the system. With 16 GB of VRAM, the
IQ3_XXS pack keeps about 35 GB of experts in RAM. gpuvm1 had 32 GiB, which
would stream part of every pack from disk. The GPU is passed through to gpuvm1
and shared by the gpu-switch workloads.

Upstream runs the container as root and expects unlimited `memlock`, 2 GiB of
shared memory, no memory limit and two minutes to unload. Pods on gpuvm1
inherit an 8 MiB `memlock` limit, and the Pod API cannot raise it.

## Decision

Run Strata as a fifth gpu-switch workload in `k8s/strata`, using the pinned
upstream `rocm-gfx120x` image with IQ3_XXS and vision disabled. Raise gpuvm1
to 48 GiB. Pass-through pins all guest memory, so this reserves 48 GiB on pve.

Request 36 GiB of memory without a limit so the scheduler keeps room for the
experts. Keep models and state on one `openebs-hostpath` PVC on gpuvm1. Back
`/dev/shm` with a 2 GiB memory `emptyDir`. Accept the root container on the
dedicated, tainted GPU worker.

Expose the OpenAI- and Anthropic-compatible API through the shared Gateway.
Upstream requires a bearer key outside loopback; source it from OpenBao
`secret/k8s/strata/api` through ESO.

First validate Strata under the default `memlock` limit. If it fails or pins
too little memory, raise `LimitMEMLOCK` for the k0s worker on gpuvm1 only.

## Consequences

- gpu-switch keeps the GPU and the RAM exclusive. Lemonade's 11 GiB request is
  released before Strata starts.
- The first start downloads about 76 GB and writes a 40 GB pack plus a 6 GB
  draft layer. This uses most of the remaining hostpath space on gpuvm1.
- A model change re-downloads and repacks into the same PVC. Remove the old
  model files manually to reclaim space.
- Strata releases often. Update the image tag and digest deliberately.
