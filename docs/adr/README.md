# Architecture Decision Records (ADR)

This directory records **why** we made significant, long-lived design decisions —
the context, the option we chose, the options we rejected, and the trade-offs.

## How it differs from the other docs

| Location | Captures | Lifecycle |
|----------|----------|-----------|
| `README.md` (per directory) | **What** the system currently is / how to use it | Kept up to date |
| `docs/adr/` | **Why** a decision was made | Append-only; never rewritten |
| `docs/plans/` | **What we are about to do** (steps, rollout) | Disposable once executed |

A `README` tells you the current shape; an ADR tells you how we got there, so the
rationale survives even after the README is rewritten.

## Conventions

- One decision per file: `NNNN-kebab-case-title.md` (zero-padded, sequential).
- Status is one of `Proposed`, `Accepted`, `Superseded by ADR-NNNN`, `Deprecated`.
- Records are **immutable**. When a decision changes, write a new ADR and mark the
  old one `Superseded by ADR-NNNN` (do not edit the original rationale away).
- Keep it short. Link to the implementing `docs/plans/*` and resulting `README`s
  instead of duplicating their content.

## When a plan completes

When a `docs/plans/*` plan is finished, split its content before deleting it:

1. **Resulting structure** → fold into the relevant `README.md`.
2. **Decisions and rejected alternatives** → extract here as an ADR.
3. **Step-by-step procedure / rollout order** → discard (git history retains it).

## Index

| ADR | Title | Status |
|-----|-------|--------|
| [0001](0001-service-oriented-ansible-playbook-organization.md) | Service-oriented Ansible playbook organization | Accepted |
| [0002](0002-dhcp-outside-proxmox-cluster-nodes.md) | Run DHCP outside the Proxmox / cluster nodes (on rpi4) | Accepted |
| [0003](0003-proxmox-host-log-collection-via-rsyslog-forwarding.md) | Proxmox host log collection via rsyslog forwarding to a central Vector | Accepted |
| [0004](0004-alertmanager-single-notification-hub.md) | Alertmanager as the single notification hub (metrics + logs) | Accepted |
| [0005](0005-renovate-native-automerge-over-branch-protection.md) | Renovate-native automerge instead of branch protection | Accepted |
| [0006](0006-custom-image-pipeline-monorepo-and-seaweedfs-s3.md) | Custom image pipeline — monorepo build + SeaweedFS S3 distribution | Accepted |
| [0007](0007-defer-grafana-dashboard-v2-migration.md) | Defer Grafana Dashboard v2 migration | Accepted |
| [0008](0008-caddy-https-upstream-for-self-signed-backends.md) | Caddy re-encrypts to HTTPS upstreams that enforce TLS (TrueNAS) | Accepted |
