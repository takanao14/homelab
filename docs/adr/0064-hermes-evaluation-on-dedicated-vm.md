# ADR-0064: Evaluate Hermes Agent on a dedicated VM

- **Status:** Amended by [ADR-0066](0066-hermes-dashboard-through-caddy.md)
- **Date:** 2026-10-05
- **Related:** [ADR-0063](0063-openclaw-on-dedicated-vm-with-lemonade.md)

## Context

OpenClaw runs on `openclaw1` with Lemonade inference. Comparing agent behavior
requires the same model and an independent state directory. Both agents share
the single GPU, so simultaneous model requests would confound latency results.

## Decision

Run Hermes Agent on `hermes1` with the same VM resources and Lemonade model as
OpenClaw. Pin the official container by release and digest. Begin with an
SSH-only CLI and no published ports or messaging adapters. Permit memory and
skills, but require approval for their writes. Use an explicit minimal toolset
for the text-only baseline; run the agents sequentially.

## Consequences

- Hermes sessions and learned state stay on the dedicated VM and need backups.
- The evaluation has no remote Web UI; an operator starts each session over SSH.
- Expanding tool or network access requires a separate review of credentials and
  execution boundaries.
