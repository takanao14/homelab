# AGENTS.md

## Role Definition
Act as a "senior infrastructure / platform engineer" for this project.
Prioritize maintainability and the DRY principle across all IaC layers.

## Project Overview
- Name: Homelab
- Context: Homelab build-out — Proxmox VM management, k0s/Kubernetes clusters
  (prd / sandbox; dev retired by ADR-0019), and GitOps-managed workloads.

## Tech Stack
- Infrastructure: Proxmox, Terraform, Terragrunt, Packer, Ansible, k0s,
  Kubernetes, Helm, Helmfile, ArgoCD
- Secrets: SOPS + AGE, direnv, OpenBao (External Secrets Operator in-cluster)
- CLI Tools: kubectl, k9s, kubie, terragrunt, k0sctl, helmfile, ansible-playbook,
  packer, sops
- Dependency updates: Renovate (app charts automerge; infra components manual)

## Directory Layout
- `ansible/`: Server provisioning and configuration management
  - `playbooks/` (service-oriented, see ADR-0001): `common-*` = baseline for all
    hosts, `ops-*` = operational one-shots, others = per-service
  - `roles/`, `inventories/` (per-environment group_vars)
- `docs/adr/`: Architecture Decision Records — check these before proposing
  structural changes; add a new ADR for significant decisions
- `docs/plans`, `docs/md`: symlinks to separate private repos (plans;
  Marp design slides). Both are gitignored here and may be absent. Edit them
  in place, but commit in their own repo — never here
- `k0s/`: k0s cluster bootstrap — Helmfile for core in-cluster components
  (Cilium, Longhorn, OpenEBS, etc.); environments under `k0s/env/`
- `k8s/`: ArgoCD-managed workloads; `k8s/argocd/` holds the app-of-apps root
  with per-environment values (`prd` / `sandbox`)
- `packer/`: Custom Proxmox cloud images; `push.sh` uploads to the SeaweedFS
  `cloud-images` S3 bucket
- `scripts/`: VM lifecycle (`create-vm.sh` / `remove-vm.sh` / `provision.sh`),
  GPU workload switching, OpenBao secret sync (`scripts/secrets/`)
- `tf/`: Proxmox resources via Terraform / Terragrunt
  - `k8s/` (k0s node VMs), `vm/`, `lxc/`, `cloudimage/`, `customimage/`,
    `modules/` (shared: proxmox-vm / proxmox-container / proxmox-cloudimage)
  - State: Cloudflare R2 primary + SeaweedFS backup

## Development and IaC Rules
- Communicate in Japanese. Use English in code, comments, and documentation.
- **DRY principle**: Consolidate common logic into Terraform modules; inject
  environment differences via `terragrunt.hcl`.
- **Impact awareness**: Changes under `tf/` may affect dependent VMs and
  environments; changes under `k0s/` and `k8s/` apply per environment.
- **Dry-run first, user applies**: Agents run read-only checks and dry-runs
  (`terragrunt plan`, `ansible-playbook --check`, `helmfile template`) and
  present the apply command; the user executes state-changing operations
  (apply, ansible runs against live hosts) and all git operations.
- **Secret management**: Secrets live in SOPS+AGE-encrypted `*.sops.env` /
  `*.sops.yaml` files committed in-repo (the repo is the master copy),
  decrypted via direnv. OpenBao stores only kubeconfigs, `.env` files, and
  the SOPS AGE key, moved in/out via `scripts/secrets/admin` — the encrypted
  files themselves are not mirrored there. Never hardcode or print secret
  values.
- **Format/validate**: `terraform fmt` / `terragrunt hclfmt` after HCL changes;
  lint/template-render Helmfile and Helm values changes before proposing.
  When rendering a wrapper chart, run `helm dependency update` first, never
  `helm dependency build`. `Chart.lock` and `charts/*.tgz` are gitignored
  build artifacts, so a local lock is whatever was last resolved on that
  machine and is usually behind `Chart.yaml`. `build` honours that stale lock
  and silently renders a different chart version than the one deployed;
  `update` re-resolves from `Chart.yaml`, which is what Argo CD does (it
  clones the repo, finds no lock, and resolves `Chart.yaml`).
- **Verify the values path, not just the key**: before editing a values file
  under `k8s/`, confirm Argo CD actually reads it —
  `kubectl -n argocd get application <name> -o jsonpath='{.spec.source.helm.valueFiles}'`
  (multi-source apps list files under `.spec.sources[].helm.valueFiles`).
  Render with exactly those files. Passing an extra `-f` that Argo CD does not
  pass will validate a file nothing reads.
- **Documentation**: Update `README.md` / `AGENTS.md` when structure changes;
  record significant design decisions as ADRs in `docs/adr/`.
- **Naming**: English variable names and comments; ASCII filenames.
