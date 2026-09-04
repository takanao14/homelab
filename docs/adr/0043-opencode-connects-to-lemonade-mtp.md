# ADR-0043: OpenCode connects to Lemonade Server with Gemma 4 MTP

- **Status:** Accepted
- **Date:** 2026-09-04
- **Supersedes:** [ADR-0041](0041-opencode-connects-to-ollama.md)
- **Related:** [ADR-0040](0040-select-lemonade-backend-by-model-and-concurrency.md),
  [`k8s/lemonade-server/README.md`](../../k8s/lemonade-server/README.md),
  [`opencode.json`](../../opencode.json)

## Context

ADR-0041 selected Ollama as OpenCode's only provider. Lemonade Server v11 now
serves `Gemma-4-12B-it-MTP-GGUF` from its single upstream Deployment, including
the model's MTP draft weights.

On 2026-09-04, OpenCode 1.18.20 completed the same read-only repository listing
task against both providers. The Lemonade run emitted a structured `bash` tool
call, consumed its result, and completed the agent loop. Lemonade reported the
model ready on the GPU; its child process selected Vulkan and included
`--model-draft` and `--spec-type draft-mtp`. This validates compatibility, not
a controlled performance comparison.

## Decision

**`opencode.json` uses `lemonade` as its only provider, via
`https://lemonade.prd.butaco.net/v1`, with
`Gemma-4-12B-it-MTP-GGUF` as the default model.** OpenCode sessions require
`scripts/gpu-switch.sh lemonade-server` to activate that mutually exclusive GPU
workload first.

The configuration does not retain Ollama as a fallback. A single provider keeps
the default route unambiguous while GPU workloads cannot run concurrently.

## Alternatives considered

- **Keep Ollama as the default.** Its `gemma4:12b` model also completed the tool
  call test. *Rejected*: the intended default is the Lemonade MTP serving path.
- **Configure both providers.** *Rejected*: this would expose a selectable route
  that is unavailable whenever the other GPU workload is active.

## Consequences

- OpenCode depends on `lemonade-server`, not `ollama`, being active.
- The default model uses the validated Vulkan and MTP runtime path. Backend
  selection remains a Lemonade recipe concern governed by ADR-0040.
- The external Lemonade endpoint remains unauthenticated and must stay within
  the trusted network until an API key is configured.
