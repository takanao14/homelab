# Architecture Decision Records (ADR)

This directory records **why** we made significant, long-lived design decisions —
the context, the option we chose, the options we rejected, and the trade-offs.

## How it differs from the other docs

| Location | Captures | Lifecycle |
|----------|----------|-----------|
| `README.md` (per directory) | **What** the system currently is / how to use it | Kept up to date |
| `docs/adr/` | **Why** a decision was made | Append-only; never rewritten |
| Plans repo (private) | **What we are about to do** (steps, rollout) | Disposable once executed |

A `README` tells you the current shape; an ADR tells you how we got there, so the
rationale survives even after the README is rewritten.

In-progress plans live in a **separate private repository**, not in this public
repo: planning and design exploration should not be published here. Only the
durable outcomes land in this repo — folded into a `README` or distilled into an
ADR — once a plan completes.

## Conventions

- One decision per file: `NNNN-kebab-case-title.md` (zero-padded, sequential).
- Status is one of `Proposed`, `Accepted`, `Superseded by ADR-NNNN`, `Deprecated`.
- Records are **immutable**. When a decision changes, write a new ADR and mark the
  old one `Superseded by ADR-NNNN` (do not edit the original rationale away).
- Keep it short. Link to the resulting `README`s instead of duplicating their
  content. Completed plans should be distilled into README updates and ADRs,
  then deleted.

## When a plan completes

When a plan in the private plans repo is finished, split its content before
deleting it:

1. **Resulting structure** → fold into the relevant `README.md`.
2. **Decisions and rejected alternatives** → extract here as an ADR.
3. **Step-by-step procedure / rollout order** → discard (the private repo
   history retains it).

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
| [0009](0009-longhorn-ui-exposed-through-authenticated-gateway-route.md) | Longhorn UI is exposed through an authenticated Gateway route | Accepted |
| [0010](0010-sandbox-argocd-uses-http-only-gitops-bootstrap.md) | Sandbox Argo CD uses HTTP-only GitOps bootstrap without cert-manager | Accepted |
| [0011](0011-cilium-gateway-to-envoy-gateway-migration.md) | Use Envoy Gateway for shared Gateway API ingress | Accepted |
| [0012](0012-openbao-eso-cluster-rebuild-registration.md) | Re-register rebuilt clusters with OpenBao for ESO | Accepted |
| [0013](0013-truenas-nfs-for-proxmox-shared-images.md) | TrueNAS NFS for Proxmox shared image storage | Accepted |
| [0014](0014-argocd-app-of-apps-shared-helm-chart.md) | ArgoCD App of Apps rendered from a shared Helm chart | Accepted |
| [0015](0015-headlamp-per-cluster-in-cluster-deployment.md) | Headlamp runs in-cluster per cluster instead of a central multi-cluster UI | Accepted |
| [0016](0016-cluster-label-via-default-scrape-class.md) | Cluster label via default scrape class, asymmetric per environment | Accepted |
| [0017](0017-renovate-automerge-golive-adjustments.md) | Renovate automerge go-live adjustments | Accepted |
| [0018](0018-seaweedfs-data-on-usb-ssd-directory-storage.md) | SeaweedFS data on a node3 USB SSD via Proxmox directory storage | Accepted |
| [0019](0019-merge-gpu-worker-into-prd-retire-dev-cluster.md) | Merge the GPU worker into prd and retire the dev cluster | Accepted |
| [0020](0020-tf-tree-axes-host-vs-cluster.md) | tf tree axes — host-named trees, cluster tree with per-stack host binding | Accepted |
| [0021](0021-relocate-prd-control-plane-to-node4.md) | Relocate the prd control plane to node4 via k0s backup/restore | Accepted |
| [0022](0022-commit-terraform-provider-lock-files.md) | Commit Terraform provider lock files | Accepted |
| [0023](0023-openbao-ansible-userpass-login.md) | Authenticate Ansible OpenBao operations via userpass login | Accepted |
| [0024](0024-shared-proxmox-node-inventory-for-monitoring.md) | Shared Proxmox node inventory for monitoring | Accepted |
