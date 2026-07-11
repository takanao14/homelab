# ADR-0022: Commit Terraform provider lock files

- **Status:** Accepted
- **Date:** 2026-07-11
- **Related:** [ADR-0020](0020-tf-tree-axes-host-vs-cluster.md),
  [ADR-0017](0017-renovate-automerge-golive-adjustments.md)

## Context

During the ADR-0020 `tf/` tree reorganization, several Terragrunt stacks still
had local `.terraform.lock.hcl` files pinning older `bpg/proxmox` provider
versions. Those locks conflicted with the repository-level `provider.tf`
constraint and blocked state pull/push operations until each affected stack was
manually reinitialized with an upgrade.

The repository previously ignored all `.terraform.*` files, so provider locks
were generated independently per workstation and per stack. That made provider
version drift likely, and it also hid an implicit `hashicorp/local` provider
dependency used by the VM and container modules.

## Decision

Commit one `.terraform.lock.hcl` per Terragrunt stack under `tf/`. Lock files
are refreshed with `tf/update-locks.sh`, which runs each stack through
`direnv exec`, upgrades providers, and records package hashes for both
`darwin_arm64` and `linux_amd64`.

Declare the `hashicorp/local` requirement in the shared generated provider
configuration with the `~> 2.9` constraint. Terragrunt generates
`tf/provider.tf` into every stack cache, so this keeps provider constraints in
one place and avoids duplicate `required_providers` blocks in sourced modules.

Renovate must not automerge Terraform provider updates. A provider constraint
PR is only complete after the lock files are regenerated and representative
Terragrunt plans have been reviewed.

## Consequences

- Provider versions and package hashes are reproducible across stacks instead
  of depending on whatever each local init selected.
- Provider updates produce wider diffs because every stack lock may change, but
  those diffs are explicit and reviewable.
- Stack operations are standardized on Terragrunt/OpenTofu. Direct Terraform
  execution against these stacks is not a supported workflow.
- Renovate remains useful for opening provider update PRs, but human review is
  required because lock refresh and plan checks are part of the update.
