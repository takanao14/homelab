# ADR-0059: Group Ansible playbooks by purpose

- **Status:** Accepted
- **Date:** 2026-10-02
- **Related:** [ADR-0001](0001-service-oriented-ansible-playbook-organization.md)

## Context

The flat playbook directory has become difficult to navigate as service,
shared configuration, and operational entry points have accumulated.

## Decision

Replace the filename prefixes from ADR-0001 with `services/`, `common/`, and
`ops/` directories. Keep `bootstrap.yaml` at the root and shared task files in
`tasks/`. Group Authentik configuration and Authentik/OpenBao operations into
service subdirectories. Preserve service-oriented deployment, inventory host
patterns, and role responsibilities.

Update callers and relative imports together without old-path wrappers.

## Consequences

Commands must use the new paths. Repository documentation and scripts use the
new layout; external callers must update their paths. ADRs retain historical
paths. Moving a playbook requires checking its task imports, vars files, and
`playbook_dir` references.
