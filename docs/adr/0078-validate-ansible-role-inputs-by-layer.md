# ADR-0078: Validate Ansible role inputs by layer

- **Status:** Accepted
- **Category:** Configuration & operations
- **Date:** 2026-10-21

## Context

Twenty roles validate inputs with task-level `ansible.builtin.assert`; none has
`meta/argument_specs.yml`, the idiomatic entry-point validation. Most asserts
check non-empty strings, lengths or formats of secrets, cross-field or
uniqueness rules, or runtime state (facts, command output, pinned checksums,
OpenBao seal state). Only enumerations such as `pdns_role` and
`knot_resolver_mode` are fully expressible as argument specs.

On ansible-core 2.21, role argument specs:

- treat a variable defined in `defaults/main.yaml` as provided, so
  `required: true` accepts `""` and `[]`;
- check only that a value converts to `type` and do not write the converted
  value back, so a string passes `type: list` and the role still receives a
  string;
- run as an `always`-tagged task that templates every declared variable,
  including lookups in defaults, before any role task, even under `--tags`;
- validate `include_role` call sites with the variables they pass.

## Decision

Each layer validates what it owns:

- Producers of derived data fail where the data is derived, for example
  `admin_authorized_keys` in `group_vars/all.yaml` fails on a missing key file.
- Roles validate their declared inputs with task-level asserts placed before
  the first task that uses them.
- Safety invariants sit immediately before the dangerous operation, such as the
  lockout guard before `exclusive: true` in `ssh_authorized_keys`.

`meta/argument_specs.yml` is used only for roles that other roles call through
`include_role`, currently `ssh_authorized_keys`. There it declares the
contract, makes required variables fail at the call site by leaving them out of
`defaults/main.yaml`, and keeps asserts for what specs cannot express. Other
roles are not converted.

## Consequences

- One validation convention remains for service roles; shared roles carry a
  spec that `ansible-doc -t role` can read.
- A spec never replaces a non-empty, strict-type, format, cross-field or
  runtime assert.
- Shared roles must not declare lazily templated, environment-dependent
  variables in their spec, because validation evaluates them even when the role
  body is skipped by tags.
- `var-naming[no-role-prefix]` is not applied to spec option names; unprefixed
  inputs still need their `noqa` in defaults.

## Alternatives considered

- **Specs in every role** — touches all roles to replace a handful of
  enumeration checks, duplicates the README variable tables, and gives false
  confidence through lenient type conversion.
- **Specs in new roles or roles as they are touched** — splits the repository
  between two conventions with little added validation.
- **Asserts only** — leaves shared roles without a declared contract checked at
  `include_role` call sites.
