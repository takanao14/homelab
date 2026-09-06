# homelab

Infrastructure-as-code for a personal homelab environment.

## Repository Structure

| Directory | Purpose |
|-----------|---------|
| [ansible/](ansible/README.md) | Server provisioning and configuration |
| [packer/](packer/README.md) | Custom Proxmox cloud image builds |
| [tf/](tf/README.md) | Terraform / Terragrunt for VMs, containers, and images |
| [k0s/](k0s/README.md) | Cluster bootstrap and lifecycle |
| [k8s/](k8s/README.md) | Argo CD-managed workloads |
| [scripts/](scripts/README.md) | VM lifecycle, provisioning, and integrations |
| [docs/adr/](docs/adr/README.md) | Architecture decisions |

[Service routing](docs/service-routing.md) describes ingress paths.
`docs/plans/` and `docs/md/` are optional symlinks to private plan and Marp
repositories; their changes belong in those repositories.

## Secret Management

Secrets are managed with [SOPS](https://github.com/getsops/sops) + [AGE](https://github.com/FiloSottile/age) encryption and exposed to tooling via [direnv](https://direnv.net/).

- Encrypted secrets are committed as `*.sops.env` or `*.sops.yaml` files.
- Each component directory contains a `.envrc` that decrypts secrets at shell entry using `sops --decrypt`.
- The `.sops.yaml` at the repository root defines encryption rules by file path pattern.

### Workflow

```bash
# Create or edit an encrypted secrets file
sops edit tf/.env/secrets.node1.sops.env

# direnv loads secrets automatically when entering a directory
cd tf
direnv allow   # first time only
```

Application secrets are declared in SOPS-encrypted Ansible inventory, seeded
into OpenBao, and synced by External Secrets Operator (see `k8s/eso/`).
Workstation kubeconfigs, `.env` files, and the AGE key are stored separately
through `scripts/secrets/admin/`; OpenBao does not mirror encrypted files.

### Reusing this repository

Decryption requires the matching AGE private key. For an independent deployment,
replace the encrypted files with your own secrets and configure your recipient
in `.sops.yaml` before encrypting them.

### What is encrypted vs. hardcoded

| Category | Handling |
|----------|----------|
| Passwords, API keys, tokens | Encrypted in `*.sops.env` or `*.sops.yaml` |
| Usernames | Environment variables or component defaults |
| IP addresses, domains, ports | Non-secret inventory/configuration; derive host addresses from inventory |
| Shared non-secret settings | `group_vars` or defaults |

## Tools Required

| Tool                       | Purpose                                  |
|----------------------------|------------------------------------------|
| `sops`                     | Secret encryption/decryption             |
| `age`                      | Encryption backend for SOPS              |
| `direnv`                   | Automatic environment variable loading   |
| `terraform` / `terragrunt` | Infrastructure provisioning              |
| `ansible`                  | Server configuration management          |
| `packer`                   | Building the custom Proxmox cloud images |
| `k0sctl`                   | k0s cluster lifecycle                    |
| `helmfile` / `helm`        | Kubernetes workload deployments          |
| `kubectl`                  | Kubernetes cluster interaction           |

## AI Coding Agent (OpenCode)

[OpenCode](https://opencode.ai/) is an optional terminal AI coding agent.
`opencode.json` uses the in-cluster Lemonade Server with
`Gemma-4-12B-it-MTP-GGUF` — see
[ADR-0043](docs/adr/0043-opencode-connects-to-lemonade-mtp.md).

```bash
curl -fsSL https://opencode.ai/install | bash
# or: npm install -g opencode-ai

opencode --version
```

Before running it, activate `lemonade-server` through
[GPU switching](k8s/lemonade-server/README.md#gpu-switching).
