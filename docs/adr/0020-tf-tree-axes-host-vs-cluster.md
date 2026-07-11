# ADR-0020: tf tree axes — host-named trees, cluster tree with per-stack host binding

- **Status:** Accepted (reorg executed 2026-07-11)
- **Date:** 2026-07-10
- **Related:** [ADR-0019](0019-merge-gpu-worker-into-prd-retire-dev-cluster.md),
  [ADR-0006](0006-custom-image-pipeline-monorepo-and-seaweedfs-s3.md)

## Context

The first directory level under `tf/vm`, `tf/lxc`, `tf/cloudimage` and
`tf/customimage` selects a Proxmox host, while under `tf/k8s` it selects an
environment. This was originally the same thing: each cluster lived on
exactly one host, so host trees for node1 and pve were simply named after
the environment they carried (`prd`, `dev`).

Two changes broke that equivalence:

- ADR-0019 retired the dev cluster and made `gpuvm` (on host pve) a prd
  worker, so `tf/vm/dev/gpuvm` managed a prd cluster VM under a directory
  named after a retired environment.
- The prd control-plane relocation places the prd controller on node4,
  so the prd cluster spans three hosts and can no longer be described by a
  single host-bound stack.

Because every Terragrunt stack binds to exactly one Proxmox API endpoint
(per-host credentials sourced by `.envrc` from `secrets.<name>.enc.env`),
cluster VMs on different hosts cannot share one stack. In addition, each
tree duplicated per-host `env.hcl`/`.envrc` boilerplate (up to five copies
per host), and `tf/common.hcl` keyed host networks by the stale names
`prd`/`dev`.

## Decision

Two rules define the tree:

1. **The first level is the tree's primary key.** `tf/vm`, `tf/lxc`,
   `tf/cloudimage` and `tf/customimage` use real host names (`pve`,
   `node1`–`node4`); `tf/k8s` uses cluster names (`prd`, `sandbox`).
2. **Host binding is declared by the stack.** The Proxmox endpoint comes
   from `.envrc` (per-host SOPS secrets) and node/datastore defaults from
   `env.hcl`. In `tf/k8s`, the cluster directory provides the default
   binding; a stack whose VM lives on another host carries its own
   `env.hcl` + `.envrc` (read explicitly via
   `read_terragrunt_config("${get_terragrunt_dir()}/env.hcl")`, since
   `find_in_parent_folders` skips the current directory). `tf/k8s/prd/cp1`
   (node4) and `tf/k8s/prd/gpuvm` (pve) follow this pattern.

Executed accordingly: `tf/vm/dev`→`tf/vm/pve`, `tf/k8s/dev`→`tf/k8s/sandbox`,
`tf/{cloud,custom}image/{prd→node1, dev→pve}`, `tf/vm/dev/gpuvm`→
`tf/k8s/prd/gpuvm`, `tf/common.hcl` keys renamed to host names, and the
stack-less `tf/lxc/dev` removed. Because the remote state key derives from
the repo path, all eight moved stacks had their state migrated
(`state pull` → `state push`) and verified with a plan gate.

## Consequences

- prd cluster VMs are discoverable in one place (`tf/k8s/prd/`) even though
  they span node1/node4/pve; host trees answer "what dies if this host
  dies" without a mental rename table.
- One-off stack semantics: `scripts/create-vm.sh` keeps generating
  host-first `tf/vm/<node>/<name>` stacks unchanged; only the `dev`→`pve`
  directory name changed.
- Old state keys (`vm/dev/*`, `k8s/dev/*`, `{cloud,custom}image/{prd,dev}`)
  remain in R2 until deleted; the SeaweedFS backup follows via rclone sync.
- Found and fixed while migrating: the `tf/customimage/base.hcl` checksum
  `run_cmd` piped `curl -fsS` into `tr`, masking download failures (the
  "fails fast" comment was ineffective) — fixed with `set -o pipefail`.
- Deferred (tracked in the private plans repo): renaming
  `secrets.{prd,dev}.enc.env` to host names, deduplicating per-host
  `vm_defaults` into a single source, renaming the `prd-cluster` stack
  once the controller moves out.
