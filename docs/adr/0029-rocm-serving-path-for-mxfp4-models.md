# ADR-0029: The ROCm backend serves MXFP4 models, reversing ADR-0028 for that quantization

- **Status:** Proposed
- **Date:** 2026-07-30
- **Related:** [ADR-0028](0028-lemonade-vulkan-backend-over-rocm-on-rdna4.md) (superseded in part),
  [ADR-0027](0027-gpu-workload-switching-web-ui.md),
  [`k8s/lemonade-server/README.md`](../../k8s/lemonade-server/README.md)

## Context

ADR-0028 chose the Vulkan backend as lemonade's serving path on the RX 9060 XT
(gfx1200), measuring it ~20% faster single-request and ~47% faster at four
concurrent requests than the ROCm `system`-backend Deployment. It kept
`lemonade-server-rocm` alive **only** to re-run that comparison, and recorded the
re-measurement trigger explicitly: "The decision rests on a measurement, and both
sides of it move."

That trigger has fired, from a direction the ADR did not anticipate. ADR-0028
compared the two backends on one model; the comparison turns out to be
**quantization-dependent**. On `gpt-oss-20b-mxfp4-GGUF` the ordering reverses and
ROCm wins. llama.cpp's Vulkan backend has no MXFP4-specialised kernel path
comparable to the HIP one, so an MXFP4 MoE dequantizes on a slower route under
RADV than it does under ROCm.

<!-- TODO: fill in before moving to Accepted.
     - Measured throughput, both backends, on gpt-oss-20b-mxfp4-GGUF
       (single-request and 4-concurrent, to match ADR-0028's methodology)
     - Which model ADR-0028 itself benchmarked, so the two runs are comparable
     - Whether any non-MXFP4 model is still served, i.e. whether the vulkan
       Deployment stays primary for anything -->

## Decision

**Point the OpenAI-compatible client at `lemonade-rocm.prd.butaco.net` for MXFP4
models, and treat the `lemonade-server-rocm` Deployment as a serving path rather
than a benchmark fixture.**

`opencode.json` selects `lemonade-rocm/gpt-oss-20b-mxfp4-GGUF`. Both Deployments
stay at `replicaCount: 0` and remain GPU-exclusive through `gpu-switch`
(ADR-0027), so this changes which one gets scaled up, not how many run.

Backend choice still lives in `config.json` on the recipe PVC, pinned per
Deployment by the `jq` initContainer, exactly as ADR-0028 established — so the two
Deployments can disagree about the backend without either corrupting the other.

## Consequences

- **ADR-0028's central claim is now scoped, not wrong.** Vulkan is faster on the
  model it measured; it is not faster universally. The backend is a per-model
  choice on this GPU, which means neither Deployment can be deleted.
- **The maintenance cost ADR-0028 refused to pay is now being paid.** The Forgejo
  image (`takanao/lemonade-docker`, `llamacpp-rocm` `b1302`, bundled ROCm 7
  userspace) sits on a serving path. Renovate cannot see the LAN registry or the
  Forgejo repository, so the base tag and `LLAMACPP_ROCM_VERSION` stay on human
  watch — and now a rotted image degrades serving, not just a benchmark. ADR-0028
  called this "not acceptable under a serving path"; that judgement is being
  overridden by the measurement, and the exposure should be stated rather than
  inherited silently.
- **ROCm host-version skew matters to lemonade again.** ADR-0028's consequence
  "lemonade no longer exercises ROCm at all" no longer holds, so
  [`ansible/roles/rocm`](../../ansible/roles/rocm/) upgrades gain back a consumer
  to check alongside comfyui and ollama.
- **`k8s/lemonade-server/README.md` needs reframing.** It currently states the
  rocm Deployment is "Not intended as a serving option — only for comparison",
  and labels the hostname and image as belonging to a "benchmark variant".

## Alternatives considered

- **Keep Vulkan and accept the MXFP4 penalty** — preserves ADR-0028 unchanged and
  keeps the custom image off the serving path. Rejected: the penalty is on the
  model actually in use, so this trades measured everyday throughput for a
  documentation convenience.
- **Serve MXFP4 from a non-MXFP4 quantization on Vulkan** — sidesteps the backend
  question by changing the model instead. Not evaluated; worth measuring before
  this ADR moves to Accepted, since it would let the custom image go away
  entirely.
- **Fold this into ADR-0028 as an edit** — rejected by convention: `docs/adr/`
  is append-only and a changed decision gets a new record
  ([docs/adr/README.md](README.md)).
