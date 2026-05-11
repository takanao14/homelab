# homelab

Infrastructure-as-code for a personal homelab environment.

## Repository Structure

```
homelab/
├── ansible/           # Ansible playbooks and roles (server provisioning & configuration)
│   ├── playbooks/     # Top-level playbooks (gpuvm, netbox, openbao, forgejo, etc.)
│   ├── roles/         # Reusable roles (rocm, lemonade, caddy, dnsdist, vector, etc.)
│   └── inventories/   # Inventory and group_vars per environment
├── k0s/               # k0s cluster bootstrap — Helmfile for core in-cluster components
├── k8s/               # ArgoCD-managed workloads
│   ├── argocd/        # ArgoCD root apps and ApplicationSets
│   ├── cert-manager/  # cert-manager + wildcard certificate issuers
│   ├── comfyui/       # ComfyUI (Stable Diffusion) deployment
│   ├── dev-monitoring/# Lightweight monitoring stack for the dev cluster
│   ├── eso/           # External Secrets Operator + ClusterSecretStore (OpenBao)
│   ├── externalDNS/   # ExternalDNS (PowerDNS provider)
│   ├── garage/        # Garage S3-compatible object storage (dev cluster)
│   ├── gateway/       # Gateway API (Cilium) setup
│   ├── homepage/      # Homepage dashboard
│   ├── lemonade-server/ # Lemonade LLM server (ROCm / AMD GPU)
│   ├── meshcentral/   # MeshCentral remote management
│   ├── monitoring/    # Prometheus, Grafana, exporters, and dashboards
│   ├── ollama/        # Ollama LLM server deployment
│   └── reloader/      # Stakater Reloader (auto-restart on ConfigMap/Secret changes)
└── tf/                # Terraform / Terragrunt (Proxmox VMs, LXC containers, cloud images)
    ├── cloudimage/    # OS image management
    ├── k8s/           # VM definitions for k0s cluster nodes
    ├── lxc/           # LXC container definitions
    ├── modules/       # Shared Terraform modules
    └── vm/            # General-purpose VM definitions
```

## Secret Management

Secrets are managed with [SOPS](https://github.com/getsops/sops) + [AGE](https://github.com/FiloSottile/age) encryption and exposed to tooling via [direnv](https://direnv.net/).

- Encrypted secrets are committed as `*.enc.env` or `*.enc.yaml` files.
- Each component directory contains a `.envrc` that decrypts secrets at shell entry using `sops --decrypt`.
- The `.sops.yaml` at the repository root defines encryption rules by file path pattern.

### Workflow

```bash
# Create or edit an encrypted secrets file
sops edit secrets.enc.env

# direnv loads secrets automatically when entering a directory
cd k8s/monitoring
direnv allow   # first time only
```

### Using this repository on a new machine

The `*.enc.env` files committed in this repository are encrypted with the author's AGE key and **cannot be decrypted by anyone else**. To use this repository, replace each encrypted file with your own secrets:

```bash
# Remove the existing encrypted file and create your own
sops edit k0s/secrets.dev.enc.env
sops edit k0s/secrets.prd.enc.env
```

Make sure your AGE key is listed in `.sops.yaml` before editing.

### What is encrypted vs. hardcoded

| Category | Handling |
|----------|----------|
| Passwords, API keys, tokens | Encrypted in `*.enc.env` |
| Usernames | Encrypted (treated as sensitive) |
| IP addresses, domains, ports | Hardcoded in config files |
| Shared non-secret config | Variables in group_vars or defaults |

## Tools Required

| Tool | Purpose |
|------|---------|
| `sops` | Secret encryption/decryption |
| `age` | Encryption backend for SOPS |
| `direnv` | Automatic environment variable loading |
| `terraform` / `terragrunt` | Infrastructure provisioning |
| `ansible` | Server configuration management |
| `k0sctl` | k0s cluster lifecycle |
| `helmfile` / `helm` | Kubernetes workload deployments |
| `kubectl` | Kubernetes cluster interaction |
