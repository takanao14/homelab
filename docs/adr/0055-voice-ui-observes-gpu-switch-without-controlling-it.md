# ADR-0055: Voice UI observes GPU availability without switching workloads

- **Status:** Accepted
- **Date:** 2026-09-24
- **Related:** [ADR-0027](0027-gpu-workload-switching-web-ui.md),
  [ADR-0043](0043-opencode-connects-to-lemonade-mtp.md)

## Context

The planned Butaco voice UI will stay reachable on the LAN while Lemonade may
be scaled to zero by `gpu-switch`. Starting Lemonade would stop another GPU
workload, including an active ComfyUI job. The voice UI serves family members
who cannot be expected to manage that tradeoff during a conversation.

## Decision

Keep the voice app running independently of the GPU allocation. It reads
Lemonade availability but receives no permission to operate `gpu-switch` or
scale GPU workloads. When Lemonade is unreachable, the UI reports that voice
responses are unavailable and returns to a state where the user can retry. It
does not claim to know which workload owns the GPU from a failed Lemonade
request alone. A reachable Lemonade with the requested model not ready is a
separate state.

The app uses a bounded request deadline. A model loading slowly may remain
within that deadline; an unavailable Lemonade must not leave recording or
playback controls stuck. Operators switch workloads through the existing
`gpu-switch` UI or CLI.

## Consequences

- Opening Butaco does not interrupt other GPU work.
- Voice conversation is unavailable while another workload owns the GPU.
- The application needs distinct unavailable, model-loading, timeout, and
  recovery states, verified on an iPhone before production use.
