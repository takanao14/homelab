# ADR-0042: Recover OpenBao by rebuilding from Ansible, not from snapshots

- **Status:** Accepted
- **Date:** 2026-09-04
- **Related:** [ADR-0012](0012-openbao-eso-cluster-rebuild-registration.md),
  [ADR-0023](0023-openbao-ansible-userpass-login.md),
  [`ansible/roles/openbao/README.md`](../../ansible/roles/openbao/README.md)

## Context

OpenBao's only snapshot is the pre-upgrade one in `ops-openbao_upgrade.yaml`,
written to the host being upgraded. Losing the VM loses it too, which looked
like a gap to close with scheduled snapshots and an off-host copy.

An inventory of the store showed the gap is narrower than it appears. Ten of the
fourteen KV entries are declared in SOPS and re-seeded by
`ops-openbao_seed_secrets.yaml`. The other four — both kubeconfigs,
`provision/env`, and `sops/age` — are written by hand through
`scripts/secrets/admin/`, and each has a live upstream copy: the workstation's
`~/.kube` and `~/.env`, and the AGE key escrowed outside OpenBao and outside
this repository. Policies, auth mounts and roles, userpass accounts, the KV
mount, and the static seal key are all produced by the role. Everything the
store holds already exists somewhere else.

Accidental change inside OpenBao is covered separately. The KV v2 mount runs
with the default ten versions, so `kv undelete` reverses a delete and
`kv rollback` reverses an overwrite. Only `kv metadata delete` escapes that.

## Decision

Recover OpenBao by rebuilding it from Ansible. Do not build a snapshot pipeline.

1. **Document the rebuild sequence** in the role README, including the step that
   has no home today: a fresh `operator init` issues a new root token and a new
   set of recovery keys, so SOPS must be updated before the bootstrap playbook
   can authenticate.
2. **Keep the pre-upgrade snapshot** as it is. It already covers the moment of
   highest risk at no additional cost.
3. **Reconcile declared secrets against the live store periodically**, comparing
   the `openbao_secrets` paths with a recursive `kv list`.

## Alternatives considered

- **Scheduled snapshots, an off-host copy, and a restore playbook** — the
  original proposal. *Rejected:* it protects data that is reproducible, and its
  two justifications did not survive scrutiny. Mistake recovery is already
  provided by KV v2 versioning, and off-books data is better removed by
  reconciliation than preserved by backup — the 2026-09-04 audit found three
  orphans (`k8s/headlamp/admin-token`, `k8s/headlamp/dev-kubeconfig`,
  `k8s/monitoring/pve-exporter`) and all three were dead.
- **Snapshots in Cloudflare R2** — rejected on its own terms before the above:
  OpenBao holds the Cloudflare API token, so replicating the store to Cloudflare
  places it inside the blast radius of the account it controls.
- **Proxmox VM-level backup** — captures `/etc/openbao/seal.key` in the same
  archive as the data, so one leaked backup is a full compromise. *Rejected.*

## Consequences

- Recovery is a documented sequence — init, bootstrap, configure, seed, re-inject
  the four hand-managed entries, register each cluster — rather than one restore
  command, and takes correspondingly longer.
- `kv metadata delete` has no undo. Accepted: it is only ever run deliberately.
- The rebuild path depends on the escrowed AGE key and the Git remote. Those,
  not the OpenBao host, are what must not be lost.
- Reconciliation stays manual until it is scripted. Without it, out-of-band
  writes accumulate unnoticed, which is how the three orphans arose.
