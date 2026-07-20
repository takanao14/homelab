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
│   ├── secrets.common.enc.env      # SOPS-encrypted shared secrets (committed)
│   └── secrets.{node1,node2,node3,node4,node5,pve}.enc.env  # SOPS-encrypted per-host secrets (committed)
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
│   │   └── toolbox2|toolbox3/      # Toolbox / scratch VMs
│   ├── node2/
│   │   ├── env.hcl                 # node2 VM defaults (storage: local-lvm)
│   │   ├── openbao/                # OpenBAO VM
│   │   ├── runner1/                # CI runner VM
│   │   └── vpngw/                  # VPN gateway VM
│   ├── node3/
│   │   ├── env.hcl                 # node3 VM defaults (storage: local-lvm)
│   │   └── toolbox/                # Toolbox VM
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
    │   └── syslog/                 # Vector log collector (syslog ingress)
    └── node3/
        ├── env.hcl                 # node3 LXC defaults (storage: local-lvm)
        ├── dnsserver/              # DNS container
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

Secrets are managed with SOPS and loaded per directory via `direnv` (each
component's `.envrc` decrypts the secrets file for its target node):

```bash
sops edit tf/.env/secrets.node1.enc.env
sops edit tf/.env/secrets.node2.enc.env
sops edit tf/.env/secrets.node3.enc.env
sops edit tf/.env/secrets.node4.enc.env
sops edit tf/.env/secrets.node5.enc.env
sops edit tf/.env/secrets.pve.enc.env
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

Commit `.terraform.lock.hcl` for every Terragrunt stack. The lock files keep
provider versions and package hashes consistent across local macOS operations
and Linux automation. They intentionally include hashes for both
`darwin_arm64` and `linux_amd64`.

When provider constraints change, refresh all stack locks from the repository
root:

```bash
./tf/update-locks.sh
```

The helper discovers every `terragrunt.hcl` under `tf/`, loads each stack's
environment with `direnv exec`, runs `terragrunt run -- init -upgrade`, and
then records provider hashes with `terragrunt run -- providers lock`. Review
the resulting lock diff together with the provider constraint change, and run
representative `terragrunt plan` checks before merging.

### Log collector resource rename

The central Vector collector was renamed from `syslog1` to `log1`. Its
`for_each` resource address was migrated on 2026-06-20. The component directory
remains `syslog/` so the existing backend state key does not change.

```bash
cd tf/lxc/node2/syslog
terragrunt plan
```

The plan must preserve the existing container and IP address
(`192.168.10.243`). Do not apply if it proposes creating or replacing the
container.

To apply all components in an environment at once:

```bash
cd tf/lxc/node2
terragrunt run-all apply
```

### Distributing images to all nodes

`cloudimage/` downloads stock cloud images (public mirrors) and `customimage/`
downloads Packer-built `.img` files from the SeaweedFS `cloud-images` bucket.
Both target every Proxmox node, but each node uses its own credentials (loaded
from its `.envrc` via SOPS), so `terragrunt run-all` cannot be used across nodes
— it would reuse a single node's credentials. Use the `run-all.sh` helper in
each directory instead, which runs `direnv exec <node>` per node to load the
right environment:

```bash
cd tf/cloudimage     # or tf/customimage (symlinked to the same script)
./run-all.sh plan
./run-all.sh apply   # auto-approved
```

Each Proxmox node fetches the image directly from the URL
(`proxmox_download_file`). Running many large downloads at once can overwhelm the
source (the single-node SeaweedFS LXC) and time out, so `run-all.sh` pins
terraform's parallelism to `1` by default (one download at a time) and runs
nodes serially. `customimage/` additionally enforces `-parallelism=1` via
`extra_arguments` in its shared `base.hcl`, so even a plain `terragrunt apply`
there is serial. Override when the source can take it:

```bash
PARALLELISM=4 ./run-all.sh apply   # relax terraform parallelism per node
PARALLEL=1   ./run-all.sh apply    # run nodes in parallel
```

> The script issues `terragrunt run -- <command> -parallelism=1`. The explicit
> `run --` form is required because Terragrunt 1.0 parses a trailing
> `-parallelism` flag itself and never forwards it to tofu/terraform, leaving
> downloads at the default parallelism of 10.

To deploy a single image instead of all of them, target its instance key:

```bash
cd tf/customimage/node2
terragrunt apply -target='proxmox_download_file.image["ubuntu-24.04-custom"]'
```

### FreeBSD cloud images

FreeBSD official VM images are currently published as `.qcow2.xz` / `.raw.xz`
archives. Do not add those URLs directly to `tf/cloudimage/images.hcl`: Proxmox
will store the compressed archive, and the bpg/proxmox `proxmox_download_file`
decompression option does not support `xz` (only `gz`, `lzo`, `zst`, and `bz2`).

To use a FreeBSD cloud image, import it through `packer/import-upstream.sh`.
That script downloads the official `.qcow2.xz`, verifies the upstream checksum,
decompresses it to `packer/images/freebsd-15.1-cloudinit-ufs.img`, and writes a
sidecar checksum for the decompressed object. Then publish it with
`packer/push.sh freebsd151` and consume it through `tf/customimage`.

## Architecture

- **Backend**: Cloudflare R2 (S3-compatible) remote state with native lockfile
  locking (`use_lockfile`); one state object per component directory
- **Providers**: bpg/proxmox ~> 0.111, hashicorp/local ~> 2.9
- **Tree axes (ADR-0020)**: first level = host name for `vm/` `lxc/`
  `cloudimage/` `customimage/` (pve, node1–node4), cluster name for `k8s/`
  (prd, sandbox). Each stack binds to exactly one Proxmox endpoint via its
  `.envrc` (per-host SOPS secrets); `k8s/` stacks whose VM lives on another
  host carry their own `env.hcl` + `.envrc`
- **Networking**: Configured via `common.hcl` per host (e.g. `vmbr0`, `vnets001`)
- **Storage**: pve=local-zfs, node1=data-nvme, node2/node3/node4=local-lvm; SeaweedFS data volume on node3 uses usb-ssd
```
