# Terraform / Terragrunt Infrastructure

Manages VMs, LXC containers, and cloud images on Proxmox using Terragrunt + Terraform.

## Prerequisites

| Tool | Purpose |
|------|---------|
| `terraform` | Infrastructure provisioning |
| `terragrunt` | DRY configuration and state management |
| `sops` | Secret decryption |

## Directory Structure

```
tf/
├── root.hcl                        # Terragrunt root config (generates provider / backend)
├── common.hcl                      # Shared locals (DNS servers, domain)
├── provider.tf                     # Proxmox provider definition (bpg/proxmox ~> 0.99)
├── .env/
│   ├── secrets.env.sample          # Secret template
│   ├── secrets.dev.enc.env         # SOPS-encrypted dev secrets (committed)
│   ├── secrets.prd.enc.env         # SOPS-encrypted prd secrets (committed)
│   └── secrets.prd2.enc.env        # SOPS-encrypted prd2 secrets (committed)
├── modules/
│   ├── proxmox-vm/                 # Proxmox VM module
│   ├── proxmox-container/          # Proxmox LXC container module
│   └── proxmox-cloudimage/         # Cloud image download module
├── cloudimage/
│   ├── images.hcl                  # Image definitions (Ubuntu 24.04, Rocky 9/10, Debian 13)
│   ├── dev/terragrunt.hcl          # dev: upload to pve node
│   ├── prd/terragrunt.hcl          # prd: upload to node1
│   └── prd2/terragrunt.hcl         # prd2: upload to node2
├── vm/
│   ├── dev/
│   │   ├── env.hcl                 # dev VM defaults
│   │   ├── guibox/                 # GUI box VM (4c/16GB, XRDP)
│   │   ├── testvm/                 # Test VMs x2 (Ubuntu / Rocky)
│   │   └── toolbox/                # Toolbox VM (4c/8GB)
│   └── prd2/
│       ├── env.hcl                 # prd2 VM defaults
│       └── vpngw/                  # VPN gateway VM (2c/1GB)
├── k8s/
│   ├── dev/
│   │   ├── env.hcl                 # dev k8s defaults
│   │   └── terragrunt.hcl          # control plane (2c/4GB) + worker (8c/8GB, 64GB+100GB)
│   └── prd/
│       ├── env.hcl                 # prd k8s defaults (datastore: data-nvme)
│       └── terragrunt.hcl          # control plane (2c/4GB) + worker (8c/24GB, 64GB+300GB)
└── lxc/
    ├── dev/
    │   ├── env.hcl                 # dev LXC defaults
    │   ├── caddy/                  # Caddy reverse proxy
    │   ├── dns/                    # DNS container
    │   └── syslog/                 # Syslog container
    └── prd2/
        ├── env.hcl                 # prd2 LXC defaults
        ├── dns/                    # DNS container
        └── syslog/                 # Syslog container
```

## Environment Variables

| Variable | Description |
|----------|-------------|
| `TF_VM_PASSWORD` | Initial password for VMs / containers |
| `TF_VM_SSH_PUBLIC_KEY` | SSH public key to inject |
| `PROXMOX_VE_ENDPOINT` | Proxmox API endpoint |
| `PROXMOX_VE_USERNAME` | Proxmox API username |
| `PROXMOX_VE_PASSWORD` | Proxmox API password |

Secrets are managed with SOPS:

```bash
sops edit tf/.env/secrets.dev.enc.env
sops edit tf/.env/secrets.prd.enc.env
sops edit tf/.env/secrets.prd2.enc.env
```

## Usage

```bash
cd tf/<type>/<env>/<component>
terragrunt init
terragrunt plan
terragrunt apply
```

To apply all components in an environment at once:

```bash
cd tf/lxc/dev
terragrunt run-all apply
```

## Architecture

- **Backend**: Local state (`terraform.tfstate` per component directory)
- **Provider**: bpg/proxmox ~> 0.99
- **Environment separation**: dev / prd / prd2 (per Proxmox node)
- **Networking**: dev=vnets001, prd=vnets30, prd2=vmbr0
- **Storage**: dev=local-zfs, prd=data-nvme, prd2=local-lvm
