# homelab

Infrastructure-as-code for a personal homelab environment.

## Repository Structure

```
homelab/
├── ansible/          # Ansible playbooks and roles (provisioning & configuration)
├── k0s/              # k0s Kubernetes cluster lifecycle management
├── k8s/              # Kubernetes workloads (Helmfile-based)
│   ├── argocd/       # ArgoCD root apps and core components
│   ├── cert-manager/ # cert-manager configuration
│   ├── comfyui/      # ComfyUI deployment
│   ├── externalDNS/  # ExternalDNS configuration
│   ├── gateway/      # Gateway API (Cilium) setup
│   ├── homepage/     # Homepage dashboard
│   ├── meshcentral/  # MeshCentral deployment
│   ├── monitoring/   # Prometheus, Grafana, Exporters, and Dashboards
│   └── ollama/       # Ollama LLM server deployment
└── tf/               # Terraform / Terragrunt (Proxmox VMs, LXC containers, cloud images)
    ├── cloudimage/
    ├── k8s/
    ├── lxc/
    ├── modules/
    └── vm/
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
