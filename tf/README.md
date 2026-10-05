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

```text
tf/
├── root.hcl          # Generated provider/backend; state key follows stack path
├── common.hcl        # Shared network settings
├── provider.tf       # Provider constraints
├── .env/             # Shared and per-node SOPS secrets
├── modules/          # proxmox-vm (cloudinit-snippets child), proxmox-container, proxmox-cloudimage
├── cloudimage/       # Stock images: images.hcl, base.hcl, <node>/
├── customimage/      # Packer images: images.hcl, base.hcl, <node>/
├── vm/<host>/<service>/
├── lxc/<host>/<service>/
└── k8s/<prd|sandbox>/<stack>/
```

Each stack uses a thin `terragrunt.hcl`; `env.hcl` supplies host or cluster
defaults. Image stacks use `node.hcl` for host binding and image selection.

## Environment Variables

| Variable | Description |
|----------|-------------|
| `TF_VM_PASSWORD` | Initial password for VMs / containers |
| `TF_VM_USERNAME` | Initial username for VMs / containers |
| `TF_VM_SSH_PUBLIC_KEY` | SSH public key to inject |
| `PROXMOX_VE_ENDPOINT` | Proxmox API endpoint |
| `PROXMOX_VE_USERNAME` | Proxmox API username |
| `PROXMOX_VE_PASSWORD` | Proxmox API password |
| `PROXMOX_VE_SSH_USERNAME` | Optional snippet-upload SSH user (default `root`); uses the SSH agent |

Each stack's `.envrc` loads its node-specific SOPS secrets through `direnv`.
Edit the relevant `secrets.<node>.sops.env`, for example:

```bash
sops edit tf/.env/secrets.node1.sops.env
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

VMs ignore `disk.file_id` after creation because Proxmox clears it. To rebuild
a VM from a new image, import the image on its node, then use `-replace` for
that VM in both the plan and apply commands.

### DNS LXC rebuilds

`lxc/dns-images.hcl` selects the OS template for each DNS host. Change one
`releases` entry at a time. `node2/dnsserver` and `node3/dnsserver` each
manage multiple containers. In those stacks, use Terraform's `-target` with
the selected container key for both plan and apply, and verify that exactly
one container is replaced. Never apply a plan replacing a peer. A changed
`TF_VM_SSH_PUBLIC_KEY` also forces replacement, even for hosts whose OS
template has not changed.

Download `ubuntu-26.04-standard_26.04-1_amd64.tar.zst` to the target node's
`local` template storage before planning. Rebuild `resolver2`, `resolver1`,
`ns3`, `ns2`, `ns1`, `dist2`, then `dist1`, checking DNS service and failover
after each host. Run the corresponding Ansible playbook with `--limit` after
each replacement. After rebuilding a secondary, run `playbooks/ops/pdns_sync.yaml` with
the primary and that secondary in `--limit`, then compare zone SOA serials.

After rebuilding `ns1`, run `make check`, `make preview-all`, and `make push-all`
from `homelab-private/dns-record`. DNSControl restores the home and reverse
zones and the prd/sandbox authority records. The prd and sandbox ExternalDNS
instances (`policy: sync`, 30-second interval) recreate their application
records when their clusters are running. Run `playbooks/ops/pdns_sync.yaml` for
`ns1:ns2:ns3` and compare forward and reverse answers on all three servers.
If a cluster is stopped, its application records will be absent until it
restarts; verify their return and run the sync playbook again then.

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

### Log collector

`lxc/node2/log_collector` manages `log1` (`192.168.10.243`). Its directory
determines the backend state key; migrate the state before planning if the
directory moves. An intentional OS rebuild must replace only `log1` after its
disk buffer has drained.

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
terragrunt apply -target='proxmox_download_file.image["ubuntu-24.04-base"]'
```

### FreeBSD cloud images

FreeBSD images use unsupported `xz` compression, so do not add their URLs
directly to `tf/cloudimage/images.hcl`.

Use `packer/import-upstream.sh` to verify, decompress, and checksum it; publish
with `packer/push.sh freebsd151`, then consume it through `tf/customimage`.

`proxmox-vm` accepts optional per-VM `cloud_init.user_data` and
`cloud_init.network_data` YAML strings. Each supplied payload replaces its
corresponding generated `user_account` or `ip_config`/`dns` configuration.
`cloud_init.type` selects the datasource; omitted values preserve the provider
default. `cloud_init.snippet_datastore` defaults to `local`, which must allow
snippets. Its child module uploads files using the `proxmox.snippets` provider
and returns their IDs. Do not put decrypted secrets in snippet content.

`vm/pve/freebsd` supplies NoCloud user-data and version 2 network-data using
`cloudinit.hcl`. The migrated verification VM in `freebsd/snippet-test` uses
the same module and is intentionally stopped (`started = false`). Both use
`192.168.10.183`; run only one at a time. Snippet edits apply to a guest only
after explicit VM replacement, as with generated initialization.

## Architecture

- **Backend**: Cloudflare R2 (S3-compatible) remote state with native lockfile
  locking (`use_lockfile`); one state object per component directory
- **Providers**: constraints live in `provider.tf`.
- **Tree axes (ADR-0020)**: hosts for `vm/`, `lxc/`, and image trees; clusters
  for `k8s/`. Each stack binds one Proxmox endpoint through `.envrc`; cross-host
  k8s stacks carry their own `env.hcl` and `.envrc`
- **Networking**: Configured via `common.hcl` per host (e.g. `vmbr0`, `vnets001`)
- **Storage**: pve=local-zfs, node1=data-nvme, node2/node3/node4=local-lvm;
  SeaweedFS data on node3 uses usb-ssd
