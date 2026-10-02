# ADR-0058: Select Authentik features at build time

- **Status:** Accepted
- **Date:** 2026-10-02
- **Related:** [ADR-0050](0050-single-definition-for-the-authentik-ldap-contract.md)

## Context

The Authentik role applies LDAP, Headlamp, and GPU Switch on every run. The
Proxy blueprint also owns both applications and their shared Outpost, so a
single application cannot be updated independently.

## Decision

Keep one full deployment playbook with a fixed initial feature selection.
Record that selection on the host and reject later changes. Split application
blueprints and role tasks by owner, and provide LDAP, Headlamp, and GPU Switch
playbooks for updating selected features. The shared Proxy Outpost remains one
object and derives its complete provider list from the recorded selection.

Feature removal and later addition are outside this workflow. Rebuild the VM
to change the selection.

## Consequences

- Fresh deployments can omit unused Outposts and applications.
- Updating one application does not reapply the others; Proxy updates still
  reconcile the shared Outpost with every selected provider.
- The upgrade and version audit workflows inspect only selected components.
