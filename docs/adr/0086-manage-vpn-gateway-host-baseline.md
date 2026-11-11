# ADR-0086: Manage the VPN gateway host baseline with Ansible

- **Status:** Accepted
- **Category:** Provisioning
- **Date:** 2026-11-11
- **Related:** [ADR-0032](0032-reboot-and-planned-shutdown-orchestration.md),
  [ADR-0085](0085-opt-in-service-guest-os-audits.md)

## Context

Terraform creates vpngw, but ADR-0032 limited Ansible to power sequencing.
Without an OS monitoring agent, the guest cannot participate in the OS audit.

## Decision

Move vpngw from `power_only` into the managed VM baseline, node_exporter,
Vector and timezone groups. Add its exporter endpoints to external monitoring.
Keep its existing position in power sequencing. Baseline management includes
SSH key convergence, package maintenance and participation in fleet upgrade
operations; initial deployment must review the gateway-only check result.

VPN application and network configuration remain externally managed. Collect
only SSH and node_exporter journals, not VPN application logs. This replaces
ADR-0032's vpngw-specific power-only treatment; the power_only mechanism remains.

## Consequences

- OS observations and agent availability become visible to existing monitoring.
- Baseline rollout requires a checked Ansible run and monitoring GitOps sync;
  inventory changes alone do not make the guest observable.
