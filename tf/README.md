# Terraform / Terragrunt Infrastructure

Manages VMs, LXC containers, and cloud images on Proxmox using Terragrunt + Terraform.

## Prerequisites

| Tool | Purpose |
|------|---------|
| `terraform` | Infrastructure provisioning |
| `terragrunt` | DRY configuration and state management |
| `direnv` | Per-directory environment loading (`.envrc`) |
| `sops` | Secret decryption |

## Directory Structure

```
tf/
├── root.hcl                        # Terragrunt root config (generates provider / backend)
├── common.hcl                      # Shared locals (DNS servers, domain, networks per host)
├── provider.tf                     # Provider constraints generated into every stack
├── .env/
│   ├── secrets.env.sample          # Secret template
│   ├── secrets.common.sops.env      # SOPS-encrypted shared secrets (committed)
│   └── secrets.{node1,node2,node3,node4,node5,pve}.sops.env  # SOPS-encrypted per-host secrets (committed)
├── modules/
│   ├── proxmox-vm/                 # Proxmox VM module
│   ├── proxmox-container/          # Proxmox LXC container module
│   └── proxmox-cloudimage/         # Image download module (stock + custom, proxmox_download_file)
├── cloudimage/
│   ├── images.hcl                  # Stock cloud image definitions (download URLs)
│   ├── base.hcl                    # Shared stack config (module source, inputs)
│   ├── run-all.sh                  # Download images to all nodes (serial, per-node creds)
│   └── node1|node2|node3|node4|node5|pve/  # Per host: thin terragrunt.hcl + node.hcl (node_name)
├── customimage/
│   ├── images.hcl                  # Custom image definitions (SeaweedFS cloud-images URLs)
│   ├── base.hcl                    # Shared stack config (module source, checksum pinning)
│   ├── run-all.sh                  # -> ../cloudimage/run-all.sh (symlink, shared)
│   └── node1|node2|node3|node4|node5|pve/  # Per host: thin terragrunt.hcl + node.hcl (node_name, image_keys)
├── vm/                             # Host-first: vm/<host>/<service> (non-k0s VMs)
│   ├── pve/
│   │   ├── env.hcl                 # pve VM defaults (storage: local-zfs, lab VMs: on_boot=false)
│   │   └── toolbox1/               # Toolbox VM (always-on; hosts the editor server)
│   ├── node2/
│   │   ├── env.hcl                 # node2 VM defaults (storage: local-lvm)
│   │   ├── openbao/                # OpenBAO VM
│   │   ├── runner1/                # CI runner VM
│   │   └── vpngw/                  # VPN gateway VM
│   ├── node3/
│   │   └── env.hcl                 # node3 VM defaults (storage: local-lvm; no stacks)
│   └── node4/
│       └── env.hcl                 # node4 VM defaults (no stacks yet; EliteDesk expansion)
├── k8s/                            # Cluster-first: k8s/<cluster>/<stack> (k0s node VMs, ADR-0020)
│   ├── prd/
│   │   ├── env.hcl                 # Default host binding: node1 (storage: data-nvme)
│   │   ├── workers-node1/          # worker1 @ node1
│   │   ├── workers-node5/          # worker2 @ node5 — own env.hcl + .envrc (host override)
│   │   ├── cp1/                    # k0s controller @ node4 — own env.hcl + .envrc (host override)
│   │   └── gpuvm/                  # GPU worker @ pve — own env.hcl + .envrc (host override)
│   └── sandbox/
│       ├── env.hcl                 # Host binding: pve (storage: local-zfs)
│       └── nodes-pve/              # cp1 + worker1-3, all on pve
└── lxc/                            # Host-first: lxc/<host>/<service>
    ├── node2/
    │   ├── env.hcl                 # node2 LXC defaults (storage: local-lvm)
    │   ├── caddy/                  # Caddy reverse proxy
    │   ├── dnsserver/              # DNS container
    │   ├── forgejo/                # Forgejo container
    │   ├── netbox/                 # NetBox container
    │   ├── resolver/               # Knot Resolver container (ADR-0030)
    │   └── syslog/                 # Vector log collector (syslog ingress)
    └── node3/
        ├── env.hcl                 # node3 LXC defaults (storage: local-lvm)
        ├── dnsserver/              # DNS container
        ├── resolver/               # Knot Resolver container (ADR-0030)
        └── seaweedfs/              # SeaweedFS container
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

Each stack's `.envrc` loads its node-specific SOPS secrets through `direnv`:

```bash
sops edit tf/.env/secrets.node1.sops.env
sops edit tf/.env/secrets.node2.sops.env
sops edit tf/.env/secrets.node3.sops.env
sops edit tf/.env/secrets.node4.sops.env
sops edit tf/.env/secrets.node5.sops.env
sops edit tf/.env/secrets.pve.sops.env
```

## Usage

```bash
# First level per tree: host name (vm/, lxc/, cloudimage/, customimage/)
# or cluster name (k8s/) — see ADR-0020.
cd tf/<type>/<host-or-cluster>/<component>
terragrunt init
terragrunt plan
terragrunt apply
```

### Provider lock files

Commit each stack's `.terraform.lock.hcl` with `darwin_arm64` and `linux_amd64`
hashes for consistent local and automated runs.

When provider constraints change, refresh all stack locks from the repository
root:

```bash
./tf/update-locks.sh
```

The helper discovers stacks, loads each direnv environment, upgrades providers,
and records both platform hashes. Review the lock diff and representative plans.

### Log collector resource rename

The Vector collector moved from `syslog1` to `log1` on 2026-06-20. Its directory
remains `syslog/` to preserve the backend state key.

```bash
cd tf/lxc/node2/syslog
terragrunt plan
```

The plan must preserve container `192.168.10.243`; reject replacement plans.

To apply all components in an environment at once:

```bash
cd tf/lxc/node2
terragrunt run-all apply
```

### Distributing images to all nodes

`cloudimage/` downloads stock images; `customimage/` downloads Packer images
from SeaweedFS. Because nodes use separate credentials, use `run-all.sh` instead
of cross-node `terragrunt run-all`:

```bash
cd tf/cloudimage     # or tf/customimage (symlinked to the same script)
./run-all.sh plan
./run-all.sh apply   # auto-approved
```

Images download directly to each node. To protect the SeaweedFS source,
`run-all.sh` defaults to serial nodes and Terraform parallelism `1`;
`customimage/base.hcl` also serializes direct applies. Override when safe:

```bash
PARALLELISM=4 ./run-all.sh apply   # relax terraform parallelism per node
PARALLEL=1   ./run-all.sh apply    # run nodes in parallel
```

> Terragrunt 1.0 requires `run --` to forward `-parallelism` to Terraform.

To deploy a single image instead of all of them, target its instance key:

```bash
cd tf/customimage/node2
terragrunt apply -target='proxmox_download_file.image["ubuntu-24.04-custom"]'
```

### FreeBSD cloud images

FreeBSD images use unsupported `xz` compression, so do not add their URLs
directly to `tf/cloudimage/images.hcl`.

Use `packer/import-upstream.sh` to verify, decompress, and checksum it; publish
with `packer/push.sh freebsd151`, then consume it through `tf/customimage`.

## Architecture

- **Backend**: Cloudflare R2 (S3-compatible) remote state with native lockfile
  locking (`use_lockfile`); one state object per component directory
- **Providers**: bpg/proxmox ~> 0.111, hashicorp/local ~> 2.9
- **Tree axes (ADR-0020)**: hosts for `vm/`, `lxc/`, and image trees; clusters
  for `k8s/`. Each stack binds one Proxmox endpoint through `.envrc`; cross-host
  k8s stacks carry their own `env.hcl` and `.envrc`
- **Networking**: Configured via `common.hcl` per host (e.g. `vmbr0`, `vnets001`)
- **Storage**: pve=local-zfs, node1=data-nvme, node2/node3/node4=local-lvm;
  SeaweedFS data on node3 uses usb-ssd
