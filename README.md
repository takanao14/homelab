# homelab

Infrastructure-as-code for a personal homelab environment.

## Repository Structure

```
homelab/
├── ansible/           # Ansible playbooks and roles (server provisioning & configuration)
│   ├── playbooks/     # Top-level playbooks (gpuvm, netbox, openbao, forgejo, etc.)
│   ├── roles/         # Reusable roles (rocm, lemonade, caddy, dnsdist, vector, etc.)
│   └── inventories/   # Inventory and group_vars per environment
├── packer/            # Packer templates building custom Proxmox cloud images
├── k0s/               # k0s cluster bootstrap — Helmfile for core in-cluster components
├── k8s/               # ArgoCD-managed workloads
│   ├── argocd/        # ArgoCD self-management + app-of-apps chart (ADR-0014)
│   ├── cert-manager/  # cert-manager + wildcard certificate issuers
│   ├── comfyui/       # ComfyUI (Stable Diffusion) deployment
│   ├── dev-monitoring/# Lightweight monitoring stack for the dev cluster
│   ├── envoy-gateway/ # Envoy Gateway controller + Gateway API CRDs (ADR-0011)
│   ├── eso/           # External Secrets Operator + ClusterSecretStore (OpenBao)
│   ├── externalDNS/   # ExternalDNS (PowerDNS provider)
│   ├── gateway/       # Shared Gateway API resources (GatewayClass, Gateway)
│   ├── headlamp/      # Headlamp Kubernetes Web UI (in-cluster per environment)
│   ├── homepage/      # Homepage dashboard
│   ├── lemonade-server/ # Lemonade LLM server (ROCm / AMD GPU)
│   ├── longhorn-ui/   # Authenticated route for the sandbox Longhorn UI
│   ├── meshcentral/   # MeshCentral remote management
│   ├── monitoring/    # Prometheus, Grafana, exporters, and dashboards
│   ├── ollama/        # Ollama LLM server deployment
│   ├── open-webui/    # Open WebUI values for the upstream chart
│   └── reloader/      # Stakater Reloader (auto-restart on ConfigMap/Secret changes)
├── scripts/           # VM lifecycle, provisioning, OpenBao secret sync, GPU switching
├── docs/
│   ├── adr/           # Architecture Decision Records
│   └── plans/         # Symlink to the private plans repository (may be absent)
└── tf/                # Terraform / Terragrunt (Proxmox VMs, LXC containers, cloud images)
    ├── cloudimage/    # Stock cloud image download (proxmox_download_file)
    ├── customimage/   # Deploy of Packer-built custom images via SeaweedFS S3 (proxmox_download_file)
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
sops edit tf/.env/secrets.prd.enc.env

# direnv loads secrets automatically when entering a directory
cd tf
direnv allow   # first time only
```

In-cluster secrets are not stored in this repository at all: they are served
by OpenBao and synced via External Secrets Operator (see `k8s/eso/`).

### Using this repository on a new machine

The `*.enc.env` files committed in this repository are encrypted with the author's AGE key and **cannot be decrypted by anyone else**. To use this repository, replace each encrypted file with your own secrets:

```bash
# Remove the existing encrypted file and create your own
sops edit tf/.env/secrets.prd.enc.env
```

Make sure your AGE key is listed in `.sops.yaml` before editing.

### What is encrypted vs. hardcoded

| Category | Handling |
|----------|----------|
| Passwords, API keys, tokens | Encrypted in `*.enc.env` |
| Usernames | Environment variables when needed; for k0s, `K0S_SSH_USER` defaults to the command runner |
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
