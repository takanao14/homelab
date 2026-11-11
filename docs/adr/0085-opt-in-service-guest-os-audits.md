# ADR-0085: Opt in service guests to OS recipe audits

- **Status:** Accepted
- **Category:** Observability
- **Date:** 2026-11-11
- **Related:** [ADR-0084](0084-audit-guest-os-creation-recipes-after-migrations.md)

## Context

Auditing every declared guest makes stopped, disposable and compatibility-test
VMs repeatedly report incomplete coverage. Boot settings do not identify an
operational service, and separate hostname lists duplicate Terraform inventory.

## Decision

Add a per-guest `os_recipe_audit` boolean, default false, to VM and LXC inputs.
Filter `guest_os_recipes` to opted-in guests; return null when no guests are
selected so the root output is omitted from state after apply. Initially opt in production
Kubernetes nodes and infrastructure service guests; leave sandbox, disposable,
compatibility and agent-evaluation guests excluded. Opted-in guests that are
unobserved remain incomplete. Empty target sets must not report success.

Use Bash for the offline CLI and jq for validated JSON comparison. CI runs
fixture tests and mock-provider module tests only. Production plan/apply and
named-output export remain local operator actions. A local wrapper collects only
named outputs and OS observations, then invokes the offline comparison; it does
not initialize or apply Terraform. A missing named output or an empty map skips
the stack, with an explicit notice. Retrieval failures remain errors. Thus only
already-published state outputs participate, not unapplied target declarations.

## Consequences

- Mixed-use stacks select guests independently without a second hostname list.
- Target changes require output refresh and new snapshots before an audit.
- This refines ADR-0084's scope and implementation, not its recipe/OS distinction.
