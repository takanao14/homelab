# Packer — Custom Cloud Images for Proxmox VE

Build cloud-init-enabled VM images for Proxmox VE.

The build and the Proxmox registration are **decoupled** through S3 (SeaweedFS):

1. `build.sh` builds the image into `images/` and writes a `.sha256` sidecar.
   For upstream images that only need normalization, `import-upstream.sh`
   downloads, verifies, and normalizes them into the same `images/` contract.
2. `push.sh` uploads the image and its checksum to the SeaweedFS `cloud-images`
   bucket (`https://s3.home.butaco.net/cloud-images/`).
3. The Terragrunt stack in [`../tf/customimage`](../tf/customimage) makes each
   Proxmox node **download** the image from that URL (`proxmox_download_file`),
   pinned by the published sha256.

Builds run on the on-demand `imagebuilder1` KVM/libguestfs VM. The controller
starts it, submits a systemd job and can disconnect without stopping the build.
The VM shuts down after ten idle minutes, including after a failed job. The
apply host needs bucket access, and Proxmox nodes download published artifacts.

## On-demand builds

Provision the VM and controller credentials as described in the
[imagebuilder role](../ansible/roles/imagebuilder/README.md). From the repository
root, with credentials injected from SOPS into the controller environment:

```bash
# Build and smoke-test a committed revision without publishing.
./scripts/image-build.sh run ubuntu26-base --revision "$(git rev-parse HEAD)"

# Publish only after excluding concurrent customimage downloads, plans and applies.
./scripts/image-build.sh run ubuntu26-base --revision "$(git rev-parse HEAD)" --publish

./scripts/image-build.sh status
./scripts/image-build.sh logs <job-id>
./scripts/image-build.sh publish-retry <job-id>
./scripts/image-build.sh shutdown
```

`run` waits for job finalization; VM shutdown follows independently after the
idle grace period. `--detach` returns the job ID immediately. `status` does not
start a stopped VM; `logs` and `publish-retry` do. Diagnostics contain only
approved lifecycle fields, never raw build output. `all` selects the existing
Packer build set; upstream-only targets such as `freebsd151` use the import path.

The CLI uses `IMAGEBUILDER_PROXMOX_TOKEN` in Proxmox's `user@realm!token=secret`
format, and discovers the VM ID by name unless `IMAGEBUILDER_VMID` is set.
Grant only `VM.Audit` and `VM.PowerMgmt` on this VM to that token, including
both user and token ACLs when privilege separation is enabled. Defaults are
`IMAGEBUILDER_PROXMOX_URL=https://192.168.10.10:8006`, `IMAGEBUILDER_NODE=pve`,
`IMAGEBUILDER_SSH_HOST=192.168.20.25`, and
`IMAGEBUILDER_SSH_USER=imagebuilder-control`. Trust the Proxmox CA through
`IMAGEBUILDER_PROXMOX_CA_FILE` when it is not in the system trust store. SSH
requires a verified host key in the controller's known_hosts and never forwards
the controller's agent. The revision must already be fetchable from the remote.

Build targets require `PKR_VAR_user_password`; publication additionally requires
the S3 variables listed below. Only these allowlisted credentials cross SSH,
through stdin into a job-specific tmpfs directory. The worker creates its own
temporary SSH key and agent for the nested guest.
Store the controller token with the build credentials in
`packer/.env/secrets.sops.env` and load them through `packer/.envrc`.

Publication verifies and retains existing image/checksum pairs before replacing
them. These two objects are not an atomic update: keep consumer operations
excluded until publication or recovery completes. `publication_blocked=true`
requires `publish-retry` for that job before new builds are accepted. Never
delete that job's artifacts or remote `.imagebuilder-backups/<job-id>/` while
recovery is pending. Remote backups are not automatically pruned.

For maintenance, run `image-build.sh start --keep-running`, then the
`ops/imagebuilder_maintenance.yaml` playbook. Maintenance persists across reboots
to keep job admission disabled while package and kernel updates finish.
Use `image-build.sh maintenance off` to resume builds and idle shutdown.
Fleet startup leaves the VM off; fleet shutdown first disables admission and
waits up to ten minutes for a running job, aborting if it cannot finish.
After recovery, explicitly clear maintenance before accepting new builds.
Do not exclude `imagebuilder1` with `--limit` when stopping its dependencies;
the admission and idle checks must run before those services stop.

## Requirements

### Build Requirements
- Packer >= 1.15.0
- QEMU tools (`qemu-img`)
- Linux amd64 builder with KVM and libguestfs (see the imagebuilder role)
- Internet access for downloading base images and packages
- For upstream imports: `curl`, `xz`, and `sha256sum`

### Upload (push to S3)

- `rclone`
- SeaweedFS S3 credentials with write access to the `cloud-images` bucket,
  exported as `SEAWEEDFS_S3_ENDPOINT` / `SEAWEEDFS_S3_ACCESS_KEY` /
  `SEAWEEDFS_S3_SECRET_KEY` (inject via `.envrc` / sops — never hardcode).

## Directory Structure

```
.
├── cinit/              # Cloud-init configuration templates for Packer
├── images/             # Generated image output directory (*.img files)
├── output-*/           # Packer build artifacts (temporary, gitignored)
├── scripts/
│   ├── common/         # Distro-agnostic provisioners (shared toolchain)
│   ├── ubuntu/         # Shell provisioners for Ubuntu
│   ├── rocky/          # Shell provisioners for Rocky Linux
│   └── debian/         # Shell provisioners for Debian
├── vars/               # Per-target inputs (URL, checksum, user, profile, scripts)
├── image.pkr.hcl       # Shared template for every target
├── build.sh            # Main build script (selects the target's var file)
├── import-upstream.sh  # Download/verify/normalize upstream compressed images
├── push.sh             # Upload built images + checksums to SeaweedFS S3
└── setup.sh            # Entry point for the Ansible build-host setup
```

## Quick Start

### 1. Set Environment Variables

```bash
# Required cloud-init password; inject through SOPS for normal builds.
export PKR_VAR_user_password='your_secure_password'

# Optional: SSH public key for the default user. When unset, templates read
# the builder's ~/.ssh/id_ed25519.pub (CI passes a stub for validate).
export PKR_VAR_ssh_pubkey='ssh-ed25519 AAAA...'

# Required for push.sh: SOPS-managed upload credentials.
export SEAWEEDFS_S3_ENDPOINT='https://s3.home.butaco.net'
export SEAWEEDFS_S3_ACCESS_KEY='...'
export SEAWEEDFS_S3_SECRET_KEY='...'
```

### 2. Build Images

```bash
./build.sh ubuntu24-base
```

Choose other targets from [Available Build Targets](#available-build-targets).
For FreeBSD, use [Upstream Image Imports](#upstream-image-imports).

### 3. Push Images to S3

```bash
# Upload one target (image + .sha256) to the cloud-images bucket
./push.sh ubuntu24-base

# Or upload every image currently in images/
./push.sh all
```

### 4. Deploy Images to Proxmox

Load node credentials through [tf/](../tf/README.md#environment-variables).
`terragrunt apply` makes each Proxmox node download the pushed images. The
checksum is read from the bucket at plan time, so an image must be pushed
(step 3) before it can be deployed.

```bash
cd ../tf/customimage/node2
terragrunt apply
```

## Image Roles

Every target is named `<distro><version>-<role>`. The role suffix says how much
is baked into the image, and each role contains the one before it:

| Role | Contents | `machine_profile` | `install_toolchain` | Disk |
|------|----------|-------------------|---------------------|------|
| `base` | QEMU Guest Agent and the timezone (JST) only | `server` | `false` | 10G |
| `tool` | `base` plus the shared CLI toolchain in `/usr/local` | `server` | `true` | 16G |
| `desktop` | `tool` plus XFCE, XRDP, the Japanese IME and the GUI applications | `desktop` | `true` | 20G |

`desktop` is reachable over RDP only through XRDP.

## Available Build Targets

Build every target from [image.pkr.hcl](image.pkr.hcl). URLs, checksums,
users, cloud-init paths, the machine profile and the provisioner list live in
`vars/<target>.pkrvars.hcl`.

| Target | Profile | Var file | Output |
|--------|---------|----------|--------|
| `ubuntu24-base` | server | [vars/ubuntu24-base.pkrvars.hcl](vars/ubuntu24-base.pkrvars.hcl) | `images/ubuntu-24.04-base.img` |
| `ubuntu24-tool` | server | [vars/ubuntu24-tool.pkrvars.hcl](vars/ubuntu24-tool.pkrvars.hcl) | `images/ubuntu-24.04-tool.img` |
| `ubuntu24-desktop` | desktop | [vars/ubuntu24-desktop.pkrvars.hcl](vars/ubuntu24-desktop.pkrvars.hcl) | `images/ubuntu-24.04-desktop.img` |
| `ubuntu26-base` | server | [vars/ubuntu26-base.pkrvars.hcl](vars/ubuntu26-base.pkrvars.hcl) | `images/ubuntu-26.04-base.img` |
| `ubuntu26-tool` | server | [vars/ubuntu26-tool.pkrvars.hcl](vars/ubuntu26-tool.pkrvars.hcl) | `images/ubuntu-26.04-tool.img` |
| `ubuntu26-desktop` | desktop | [vars/ubuntu26-desktop.pkrvars.hcl](vars/ubuntu26-desktop.pkrvars.hcl) | `images/ubuntu-26.04-desktop.img` |
| `rocky10-base` | server | [vars/rocky10-base.pkrvars.hcl](vars/rocky10-base.pkrvars.hcl) | `images/rocky-10-base.img` |
| `rocky10-tool` | server | [vars/rocky10-tool.pkrvars.hcl](vars/rocky10-tool.pkrvars.hcl) | `images/rocky-10-tool.img` |
| `rocky9-base` | server | [vars/rocky9-base.pkrvars.hcl](vars/rocky9-base.pkrvars.hcl) | `images/rocky-9-base.img` |
| `rocky9-tool` | server | [vars/rocky9-tool.pkrvars.hcl](vars/rocky9-tool.pkrvars.hcl) | `images/rocky-9-tool.img` |
| `rocky9-desktop` | desktop | [vars/rocky9-desktop.pkrvars.hcl](vars/rocky9-desktop.pkrvars.hcl) | `images/rocky-9-desktop.img` |
| `debian13-base` | server | [vars/debian13-base.pkrvars.hcl](vars/debian13-base.pkrvars.hcl) | `images/debian-13-base.img` |

Debian has no `tool` target: the toolchain's HashiCorp step resolves an apt
suite from `VERSION_CODENAME` and `apt.releases.hashicorp.com` publishes no `trixie`
suite. Rocky 10 has no `desktop` target because nothing consumes one.

### Rocky base images need CRB before provisioning

Rocky `base` targets omit `scripts/rocky/epel.sh`. Before running
`scripts/provision.sh` on them, enable EPEL and CRB: the vendored package
installer enables EPEL only, while packages such as `mosh` need CRB dependencies.

```bash
# Rocky 10 ships dnf5, whose config-manager syntax differs from dnf4
sudo dnf install -y epel-release
sudo dnf install -y dnf5-plugins && sudo dnf config-manager setopt crb.enabled=1
# Rocky 9:
# sudo dnf install -y dnf-plugins-core && sudo dnf config-manager --set-enabled crb
```

Ubuntu and Debian `base` images need no equivalent step.

To add a target, create a `vars/<target>.pkrvars.hcl` and register the target in
`build.sh`, `push.sh` and `tf/customimage/images.hcl`
(`scripts/check-image-refs.sh` verifies the filename mapping in CI).

## Machine Profiles

Packer writes `machine_profile` to `/etc/provisioning/machine-profile.local`
and passes it to shared installers as `TOOL_MACHINE_PROFILE` so later
`scripts/provision.sh` runs retain the image's server or desktop profile when
installing missing tools. Rocky base hosts require the repository setup above.

Only `desktop` installs Freelens, kitty, UDEV Gothic NF, and shared kitty
settings under `/etc/xdg/kitty/kitty.conf`. Per-user preferences belong to
the dotfiles repository.

GUI applications that are not part of the shared toolchain (Firefox, VS Code,
Wireshark, virt-manager) come from the distro provisioner lists
(`scripts/<distro>/tools.sh`, `vm.sh`), which only the `-desktop` targets
include.

The CLI toolchain is installed system-wide under `/usr/local/share/mise` with
its config and lockfile in `/etc/mise`. Per-user mise shims precede the system
shims, allowing project-specific mise configs to override the image baseline.
`mise install --locked` consumes committed Linux artifact URLs and checksums
without live release resolution.

## Upstream Image Imports

Some upstream images require normalization but no Packer build.

FreeBSD publishes `.qcow2.xz`, which `proxmox_download_file` cannot decompress.
Import it into the standard image/checksum contract:

```bash
./import-upstream.sh freebsd151
```

The import verifies the pinned upstream archive checksum, decompresses it to
`images/freebsd-15.1-cloudinit-ufs.img`, and writes a `.sha256` sidecar for the
decompressed artifact that Proxmox downloads.

Then publish it like any other custom image:

```bash
./push.sh freebsd151
```

## Build Script

```bash
./build.sh [OPTIONS] <IMAGE_TYPE>
```

**Options:**
- `-y` - Force overwrite existing images without prompting

`IMAGE_TYPE` is any target in the table above or `all`. Pair `all` with `-y`
for unattended builds. `all` excludes `ubuntu24-tool`, `ubuntu24-desktop`, and
`rocky9-tool`; these remain available as individual targets.

Upstream imports are handled separately by `import-upstream.sh`; they are not
included in `build.sh all`.

The script confirms overwrite, clears temporary output, runs Packer and
`virt-sparsify --compress`, and writes the image and `.sha256` under `images/`.
It removes raw qcow2 output after each build to keep `all` from accumulating
intermediates.

## Dependency Management

Root `renovate.json` manages tracked dependency versions.

Tool pins and the mise lockfile are maintained in `takanao14/dotfiles`.
Refresh the shared installers through `../scripts/install/vendor/sync.sh`.

APT/DNF packages installed without version pins use repository candidates
(terraform, packer, vault, Firefox, VS Code, Wireshark, Podman, etc.).

## Customization

### Manual Packer Build

Run Packer directly for custom configurations:

```bash
packer build \
  -var-file "vars/ubuntu24-base.pkrvars.hcl" \
  -var "output_directory=scratch-output" \
  -var "vm_name=scratch.qcow2" \
  -var "image_name=images/scratch.img" \
  image.pkr.hcl
```

### Modifying Provisioning Scripts

Select provisioners through each target's `vars/<target>.pkrvars.hcl`.

`qemu-ga.sh` installs the guest agent and nothing else; the timezone is a
separate `scripts/common/timezone.sh` step listed by every target. The Rocky
targets omit `qemu-ga.sh` because GenericCloud already ships the agent.

Cloud-init templates live in `cinit/`. Keep SSH password authentication disabled
and inject credentials through environment variables.

## Troubleshooting

### Build Fails with "Permission Denied"
Ensure the Packer user has sudo access in the base cloud image.

### Packer Cannot Connect to VM
Check that:
- QEMU is properly installed
- KVM is available (`/dev/kvm` exists)
- No firewall blocking SSH on port 22

### Terragrunt Apply Fails
Verify:
- Proxmox credentials are set correctly
- API endpoint is accessible
- Target node and datastore exist

### Download Fails to Resolve `s3.home.butaco.net`
Downloads run on each Proxmox node, not the apply host. If internal DNS fails,
map `s3.home.butaco.net` to Caddy (`192.168.10.244`) in that node's `/etc/hosts`.

### Download Stalls or the Image Server Restarts Mid-Transfer
Multi-GB transfers charge page cache to the SeaweedFS LXC cgroup and can OOM
`weed`. Keep sufficient RAM/swap (currently 8 GB + 4 GB) and run
`tf/customimage` with `-parallelism=1`.
