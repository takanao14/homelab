# ADR-0035: OpenCode connects to Ollama instead of Lemonade Server

- **Status:** Accepted
- **Date:** 2026-08-22
- **Related:** [ADR-0027](0027-gpu-workload-switching-web-ui.md),
  [ADR-0028](0028-lemonade-vulkan-backend-over-rocm-on-rdna4.md),
  [ADR-0029](0029-rocm-serving-path-for-mxfp4-models.md),
  [`k8s/ollama/README.md`](../../k8s/ollama/README.md),
  [`opencode.json`](../../opencode.json)

## Context

ADR-0029 recorded `opencode.json` selecting
`lemonade-rocm/gpt-oss-20b-mxfp4-GGUF` — Lemonade Server's ROCm variant — as
the only OpenAI-compatible client configuration in this repository.

`ollama` already runs the same shared GPU as a separate `homelab/gpu-switchable`
Deployment (ADR-0027), serving Open WebUI. Its chart values
(`k8s/ollama/values.yaml`) already carry `numCtx: 32768`,
`flashAttention: true`, and `kvCacheType: q8_0` — a context window and
quantized KV cache sized for long agent sessions, not just interactive chat —
so the Deployment was effectively already prepared for an agent client before
one was configured to use it.

The decision here is to standardize OpenCode's single client configuration on
Ollama and drop the Lemonade one, rather than run both as switchable
alternatives.

## Decision

**`opencode.json` uses `ollama` as its only provider, via
`https://ollama.prd.butaco.net/v1`, with model `gemma4:12b`.** The
`lemonade-rocm` provider block is removed.

`gemma4:12b` is Ollama's tag for Google's Gemma 4 12B dense model (released
2026-04, Apache 2.0). Every Gemma 4 variant ships with native tool calling,
which OpenCode's agent loop depends on. Its weights are ~6.7 GB, comfortably
under the RX 9060 XT's 16 GB VRAM alongside the pre-existing
`numCtx: 32768` / `kvCacheType: q8_0` budget in `k8s/ollama/values.yaml`
(sized for an 8–13 GB model, per that file's comment) — leaving more headroom
than the 8–13 GB range assumed, rather than sitting at its edge.

This ADR does not revisit ADR-0028 or ADR-0029: Lemonade's Vulkan-vs-ROCm
backend measurements stand, and `lemonade-server` / `lemonade-server-rocm`
remain deployed for that benchmark/comparison role. Only the endpoint an
interactive OpenCode session points at changes.

## Alternatives considered

- **Keep `lemonade-rocm` as the OpenCode provider, add `ollama` alongside
  it.** Preserves the ADR-0029 configuration and lets a session pick either
  backend depending on which one `gpu-switch` currently has running.
  *Rejected*: a single standing client configuration is simpler to reason
  about than two providers that are mutually exclusive at the GPU level, and
  the model-catalog research behind this decision found Ollama and Lemonade
  reach the same underlying Hugging Face GGUF population, so keeping both
  configured added no real model choice — only a second endpoint to keep in
  sync.
- **Switch the default model to a Lemonade Vulkan endpoint
  (`lemonade.prd.butaco.net`)**, matching ADR-0028's single-request speed
  finding. *Not pursued*: it still depends on the Lemonade custom-image
  maintenance path (for the ROCm comparison variant) and does not reduce the
  number of standing GPU-serving stacks OpenCode depends on.
- **Reuse `gpt-oss:20b`**, the same weight family as the
  `gpt-oss-20b-mxfp4-GGUF` checkpoint ADR-0029 measured on Lemonade, so the
  model itself would carry no new unknowns. *Rejected in favor of
  `gemma4:12b`*: Gemma 4's native tool-calling support across its whole
  lineup (vs. gpt-oss's harmony/function-calling format) and its smaller
  footprint leave more VRAM headroom for the 32k-context budget already
  configured — traded against gpt-oss-20b having no prior measurement on this
  hardware via either backend.
- **`gemma4:26b`** (MoE, 3.8B active params, more capable). *Rejected*: its
  ~14.4 GB run footprint leaves too little of the 16 GB VRAM budget for the
  32k-token `q8_0` KV cache already configured; would require lowering
  `numCtx` to fit.

## Consequences

- Ollama joins the exclusive GPU rotation as OpenCode's dependency: using
  OpenCode now requires `scripts/gpu-switch.sh ollama` (or the gpu-switch UI)
  to have Ollama active first, stopping whatever else was running
  (`lemonade-server`, `lemonade-server-rocm`, `comfyui`, or `vllm`).
- `gemma4:12b` must be pulled once into the `ollama-models` PVC
  (`ollama pull gemma4:12b`, ~6.7 GB) before first use.
- `lemonade-server-rocm` is no longer exercised by any standing client. It
  stays deployed at `replicaCount: 0` for the ADR-0028/0029 benchmark role;
  nothing else currently depends on it running.
- `opencode.json` no longer names a Lemonade endpoint, so ADR-0029's line "the
  current client therefore chooses concurrent aggregate throughput" describes
  a configuration that has since changed — recorded here rather than edited
  into ADR-0029, per this directory's append-only convention.
