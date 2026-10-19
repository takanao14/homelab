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
├── lxc/            # images.hcl (per-guest LXC release), <host>/<service>/
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

Stacks inherit shared R2 and node-specific SOPS credentials through parent
`.envrc` files. Cross-host k8s stacks override the node credentials in their own
`.envrc`. Edit the relevant `secrets.<node>.sops.env`, for example:

```bash
sops edit tf/.env/secrets.node1.sops.env
```

## Usage

```bash
cd tf/<type>/<host-or-cluster>/<component>
terragrunt init
terragrunt plan
terragrunt apply
```

VMs ignore `disk.file_id` after creation because Proxmox clears it. To rebuild
a VM from a new image, import the image on its node, then use `-replace` for
that VM in both the plan and apply commands.

### DNS LXC rebuilds

`lxc/images.hcl` selects every LXC guest's OS template. Change one DNS
`releases` entry at a time. Each host has `dnsdist`, `pdns` and `resolver` stacks;
`node2/pdns` manages ns1 and ns3, so use `-target` for the selected container in
both plan and apply there, and verify that exactly one container is replaced.
Never replace a peer in the same apply.
Changing `TF_VM_SSH_PUBLIC_KEY` also replaces containers with unchanged templates.

Download the selected template to the node's `local` template storage first.
All seven DNS guests use Debian 13. After each replacement, run bootstrap and
the service's Ansible playbook with `--limit` and check DNS service and failover.
For a resolver, also run `playbooks/services/blackbox_exporter.yaml` with the same
limit to restore root-server reachability probes. After a secondary rebuild, run
`playbooks/ops/pdns_sync.yaml` limited to the primary and that secondary and
compare SOA serials.

After rebuilding `ns1`, run `make check`, `make preview-all`, and `make push-all`
from `homelab-private/dns-record`. DNSControl restores home/reverse zones,
prd/sandbox authority records, and the sandbox wildcard. prd ExternalDNS
recreates application records when its cluster runs. Run
`playbooks/ops/pdns_sync.yaml` for `ns1:ns2:ns3` and compare forward/reverse answers
and SOA serials. If prd is stopped, verify the application records return after
startup and repeat the sync.

### Provider lock files

Commit each stack's `.terraform.lock.hcl` with `darwin_arm64` and `linux_amd64`
hashes for consistent local and automated runs.

When provider constraints change, refresh all stack locks from the repository
root:

```bash
./tf/update-locks.sh
```

The helper refreshes every stack lock with both platform hashes. Review the
lock diff and representative plans.

### Log collector

`lxc/node2/log_collector` manages `log1` (`192.168.10.243`). Its directory
determines the backend state key; migrate the state before planning if the
directory moves. An intentional OS rebuild must replace only `log1` after its
disk buffer has drained.

To apply all stacks under one node:

```bash
cd tf/lxc/node2
terragrunt run --all -- apply
```

### Distributing images to all nodes

`cloudimage/` downloads stock images and `customimage/` downloads Packer images
from SeaweedFS directly to each node. Use `run-all.sh` to load each node's
credentials; cross-node `terragrunt run --all` shares one environment.

```bash
cd tf/cloudimage     # or tf/customimage (symlinked to the same script)
./run-all.sh plan
./run-all.sh apply   # auto-approved
```

To protect the SeaweedFS source, `run-all.sh` defaults to serial nodes and
Terraform parallelism `1`; `customimage/base.hcl` also serializes direct applies.
Override when safe:

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

`proxmox-vm` accepts `cloud_init.user_data` and `cloud_init.network_data` YAML
strings, replacing generated `user_account` and `ip_config`/`dns` settings
respectively. `cloud_init.type` selects the datasource; omission preserves the
provider default. The `cloudinit-snippets` child module uploads payloads through
`proxmox.snippets` using SSH. `cloud_init.snippet_datastore` defaults to `local`,
which must support snippets. Never include decrypted secrets in the payloads.

`vm/pve/freebsd` supplies NoCloud user-data and version 2 network-data through
`cloudinit.hcl`. Snippet changes require explicit VM replacement to reach the
guest, as with generated initialization.

## Architecture

- **Backend**: Cloudflare R2 (S3-compatible) remote state with native lockfile
  locking (`use_lockfile`); one state object per component directory
- **Providers**: constraints live in `provider.tf`.
- **Stack binding (ADR-0020)**: each stack uses one Proxmox endpoint from
  `.envrc`; cross-host k8s stacks override inherited host defaults and credentials.
- **Networking**: Configured via `common.hcl` per host (e.g. `vmbr0`, `vnets001`)
- **Storage**: pve=local-zfs, node1/node5=data-nvme, node2/node3/node4=local-lvm;
  SeaweedFS data on node3 uses usb-ssd
