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
├── common.hcl                      # Shared locals (DNS servers, domain, networks per env)
├── provider.tf                     # Proxmox provider definition (bpg/proxmox ~> 0.111)
├── .env/
│   ├── secrets.env.sample          # Secret template
│   ├── secrets.common.enc.env      # SOPS-encrypted shared secrets (committed)
│   └── secrets.{dev,prd,node2,node3,node4}.enc.env  # SOPS-encrypted per-node secrets (committed)
├── modules/
│   ├── proxmox-vm/                 # Proxmox VM module
│   ├── proxmox-container/          # Proxmox LXC container module
│   └── proxmox-cloudimage/         # Image download module (stock + custom, proxmox_download_file)
├── cloudimage/
│   ├── images.hcl                  # Stock cloud image definitions (download URLs)
│   ├── base.hcl                    # Shared stack config (module source, inputs)
│   ├── run-all.sh                  # Download images to all nodes (serial, per-node creds)
│   └── dev|prd|node2|node3|node4/  # Per node: thin terragrunt.hcl + node.hcl (node_name)
├── customimage/
│   ├── images.hcl                  # Custom image definitions (SeaweedFS cloud-images URLs)
│   ├── base.hcl                    # Shared stack config (module source, checksum pinning)
│   ├── run-all.sh                  # -> ../cloudimage/run-all.sh (symlink, shared)
│   └── dev|prd|node2|node3|node4/  # Per node: thin terragrunt.hcl + node.hcl (node_name, image_keys)
├── vm/
│   ├── dev/
│   │   ├── env.hcl                 # dev VM defaults (node: pve, storage: local-zfs)
│   │   ├── gpuvm/                  # GPU passthrough VM (Ollama)
│   │   └── toolbox2|toolbox3/      # Toolbox / scratch VMs
│   ├── node2/
│   │   ├── env.hcl                 # node2 VM defaults (node: node2, storage: local-lvm)
│   │   ├── openbao/                # OpenBAO VM
│   │   ├── runner1/                # CI runner VM
│   │   └── vpngw/                  # VPN gateway VM
│   └── node3/
│       ├── env.hcl                 # node3 VM defaults (node: node3, storage: local-lvm)
│       └── toolbox/                # Toolbox VM
├── k8s/
│   ├── dev/
│   │   ├── env.hcl                 # dev k8s defaults (node: pve, storage: local-zfs)
│   │   └── dev-cluster/
│   └── prd/
│       ├── env.hcl                 # prd k8s defaults (node: node1, storage: data-nvme)
│       └── prd-cluster/
└── lxc/
    ├── dev/
    │   └── env.hcl                 # dev LXC defaults (node: pve, storage: local-zfs)
    ├── node2/
    │   ├── env.hcl                 # node2 LXC defaults (node: node2, storage: local-lvm)
    │   ├── caddy/                  # Caddy reverse proxy
    │   ├── dnsserver/              # DNS container
    │   ├── forgejo/                # Forgejo container
    │   ├── netbox/                 # NetBox container
    │   └── syslog/                 # Vector log collector (syslog ingress)
    └── node3/
        ├── env.hcl                 # node3 LXC defaults (node: node3, storage: local-lvm)
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
- **Provider**: bpg/proxmox ~> 0.111
- **Environment separation**: dev / prd / node2 / node3 / node4 (per Proxmox node)
- **Networking**: Configured via `common.hcl` per environment (e.g. `vmbr0`, `vnets001`)
- **Storage**: dev=local-zfs (pve), prd=data-nvme (node1), node2=local-lvm, node3=local-lvm; SeaweedFS data volume on node3 uses usb-ssd
```
