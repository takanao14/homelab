# ADR-0084: Audit guest OS creation recipes after migrations

- **Status:** Accepted
- **Category:** Observability
- **Date:** 2026-11-10
- **Related:** [ADR-0077](0077-select-guest-os-by-role.md)

## Context

VM image IDs and LXC templates describe creation recipes, not running OS state.
An apply can update recipe metadata without updating a guest OS. Publishing Git
changes as monitoring expectations would also warn during intentionally deferred
rollouts. Reinterpreting HCL outside Terraform duplicates its evaluation rules.

## Decision

Perform an on-demand recipe-alignment audit after migration acceptance. Terraform
modules expose only guest names, placement, kind, and the resolved creation image.
The VM image is the first disk in the same sorted order used by its resource.
The output is an applied recipe snapshot, not evidence of OS update completion.

Compare these snapshots with existing `node_os_info`/`node_uname_info` observations.
Infer recipe releases from the repository's image naming convention (ADR-0038).
Report unknown images, unobserved guests and conflicting observations as incomplete;
do not silently treat them as matches. The audit never runs Terragrunt or mutates
infrastructure. Production output export remains an operator action.

Keep dashboards and the allowed-release alert as operational monitoring. Continuous
drift notifications require a separate approved operating-OS baseline and rules
for migration acceptance, maintenance and exceptions; creation images alone are
insufficient for that responsibility.

## Consequences

- Output additions require operator plan/apply before export. No guest resource
  configuration or automatic remediation is introduced.
- Unmonitored/offline guests remain incomplete until separately observed.
- Opaque or differently named images need an explicit extension to the audit.
