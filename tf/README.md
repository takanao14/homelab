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
├── common.hcl                      # Shared locals (DNS servers, domain, networks per env)
├── provider.tf                     # Proxmox provider definition (bpg/proxmox ~> 0.105)
├── .env/
│   ├── secrets.env.sample          # Secret template
│   ├── secrets.dev.enc.env         # SOPS-encrypted dev secrets (committed)
│   ├── secrets.prd.enc.env         # SOPS-encrypted prd secrets (committed)
│   ├── secrets.node2.enc.env       # SOPS-encrypted node2 secrets (committed)
│   └── secrets.node3.enc.env       # SOPS-encrypted node3 secrets (committed)
├── modules/
│   ├── proxmox-vm/                 # Proxmox VM module
│   ├── proxmox-container/          # Proxmox LXC container module
│   └── proxmox-cloudimage/         # Cloud image download module
├── cloudimage/
│   ├── images.hcl                  # Image definitions (Ubuntu 24.04, Rocky 9, Debian 13)
│   ├── dev/terragrunt.hcl          # dev: upload to pve node
│   ├── prd/terragrunt.hcl          # prd: upload to node1
│   ├── node2/terragrunt.hcl        # node2: upload to node2
│   └── node3/terragrunt.hcl        # node3: upload to node3
├── vm/
│   ├── dev/
│   │   ├── env.hcl                 # dev VM defaults (node: pve, storage: local-zfs)
│   │   ├── gpuvm/                  # GPU VM (8c/32GB, 200GB+300GB, PCIe passthrough: radeon)
│   │   ├── guibox/                 # GUI box VM (XRDP)
│   │   ├── testvm/                 # Test VMs x2 (testvm1: Ubuntu 4c/8GB, testvm2: Rocky 4c/8GB)
│   │   └── toolbox/                # Toolbox VM
│   └── node2/
│       ├── env.hcl                 # node2 VM defaults (node: node2, storage: local-lvm)
│       ├── openbao/                # OpenBao secret management VM (1c/1GB, 16GB)
│       ├── runner1/                # Forgejo runner VM (2c/4GB, 40GB)
│       └── vpngw/                  # VPN gateway VM (2c/1GB, 10GB, Debian)
├── k8s/
│   ├── dev/
│   │   ├── env.hcl                 # dev k8s defaults (node: pve, storage: local-zfs)
│   │   ├── dev-cluster/            # dev k8s: cp1 (2c/4GB) + worker1 (8c/8GB, 64GB+100GB)
│   │   └── sandbox/                # sandbox k8s: cp1 + workers 1-3 (all 2c/4GB, 40GB+40GB)
│   └── prd/
│       ├── env.hcl                 # prd k8s defaults (node: node1, storage: data-nvme)
│       └── prd-cluster/            # prd k8s: cp1 (2c/4GB) + worker1 (8c/24GB, 64GB+300GB)
└── lxc/
    ├── dev/
    │   └── env.hcl                 # dev LXC defaults (node: pve, storage: local-zfs)
    ├── node2/
    │   ├── env.hcl                 # node2 LXC defaults (node: node2, storage: local-lvm)
    │   ├── caddy/                  # Caddy reverse proxy
    │   ├── dnsserver/              # DNS container
    │   ├── forgejo/                # Forgejo container
    │   ├── netbox/                 # NetBox container
    │   └── syslog/                 # Syslog container
    └── node3/
        ├── env.hcl                 # node3 LXC defaults (node: node3, storage: local-lvm)
        └── dnsserver/              # DNS container
```

## Environment Variables

| Variable | Description |
|----------|-------------|
| `TF_VM_PASSWORD` | Initial password for VMs / containers |
| `TF_VM_USERNAME` | Initial username for VMs / containers |
| `TF_VM_SSH_PUBLIC_KEY` | SSH public key to inject |
| `PROXMOX_VE_ENDPOINT` | Proxmox API endpoint |
| `PROXMOX_VE_USERNAME` | Proxmox API username |
| `PROXMOX_VE_PASSWORD` | Proxmox API password |

Secrets are managed with SOPS:

```bash
sops edit tf/.env/secrets.dev.enc.env
sops edit tf/.env/secrets.prd.enc.env
sops edit tf/.env/secrets.node2.enc.env
sops edit tf/.env/secrets.node3.enc.env
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
cd tf/lxc/node2
terragrunt run-all apply
```

## Architecture

- **Backend**: Local state (`terraform.tfstate` per component directory)
- **Provider**: bpg/proxmox ~> 0.105
- **Environment separation**: dev / prd / node2 / node3 (per Proxmox node)
- **Networking**: Configured via `common.hcl` per environment (e.g. `vmbr0`, `vnets001`)
- **Storage**: dev=local-zfs (pve), prd=data-nvme (node1), node2=local-lvm, node3=local-lvm
