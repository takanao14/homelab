# ADR-0063: Run OpenClaw on a dedicated VM with Lemonade inference

- **Status:** Accepted
- **Date:** 2026-10-04
- **Related:** [ADR-0027](0027-gpu-workload-switching-web-ui.md), [ADR-0043](0043-opencode-connects-to-lemonade-mtp.md), [ADR-0060](0060-mcp-relies-on-bearer-authentication.md)

## Context

OpenClaw needs persistent Gateway state and a browser UI. Lemonade already
serves the selected Gemma model on the prd GPU. The single GPU is shared through
gpu-switch, so inference is unavailable whenever another workload is selected.
The shared Envoy Gateway does not reliably preserve client IPs for source CIDR
policies (ADR-0060).

## Decision

Run one OpenClaw Gateway in a dedicated `openclaw1` VM on pve. Use the pinned
upstream image under Podman and keep its state on the VM. Connect to the existing
Lemonade HTTPS endpoint with the OpenAI-compatible API. Keep Lemonade's current
unauthenticated API contract; its client is a placeholder API key required by
OpenClaw's provider adapter, not a server credential.

Start with the Web UI only, a minimal tool profile, and Gateway token auth.
Publish the Gateway on VM loopback and its net20 address; the VM firewall
restricts the net20 port to Caddy. Operators reach the HTTPS UI through Caddy
over VPN or through SSH forwarding for recovery. Keep Lemonade selected in
gpu-switch for normal operation; manually
reselect it after another GPU workload or a planned shutdown.

## Consequences

- OpenClaw stays up during inference outages but cannot answer until Lemonade
  returns. No component automatically changes the GPU assignment.
- Back up VM state; recover the Gateway token from its SOPS source.
- Expanding to messaging channels or broader tools requires a new
  review of access and execution boundaries.
