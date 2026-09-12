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

The apply host needs bucket access, not local images. Packer can run separately
on a KVM/libguestfs builder.

## Requirements

### Build Requirements
- Packer >= 1.15.0
- QEMU tools (`qemu-img`)
- Proxmox VE API access
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
└── setup.sh            # One-time build-host setup (QEMU/KVM + libguestfs)
```

## Quick Start

### 1. Set Environment Variables

```bash
# Required: Set the default user password for cloud-init
export PKR_VAR_user_password='your_secure_password'

# Optional: SSH public key for the default user. When unset, templates read
# the builder's ~/.ssh/id_ed25519.pub (CI passes a stub for validate).
export PKR_VAR_ssh_pubkey='ssh-ed25519 AAAA...'

# Required for push.sh: SeaweedFS S3 credentials (write to cloud-images)
export SEAWEEDFS_S3_ENDPOINT='https://s3.home.butaco.net'
export SEAWEEDFS_S3_ACCESS_KEY='...'
export SEAWEEDFS_S3_SECRET_KEY='...'

# Optional: Proxmox credentials for Terragrunt deployment
export PROXMOX_API_TOKEN=apiuser@pve!provider=...
export PROXMOX_ENDPOINT=https://...
export PROXMOX_VE_SSH_USERNAME='proxmox_user'
export PROXMOX_VE_SSH_AGENT=true
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

Every target is built from [image.pkr.hcl](image.pkr.hcl). URLs, checksums,
users, cloud-init paths, the machine profile and the provisioner list live in
`vars/<target>.pkrvars.hcl`.

| Target | Profile | Var file | Output |
|--------|---------|----------|--------|
| `ubuntu24-base` | server | [vars/ubuntu24-base.pkrvars.hcl](vars/ubuntu24-base.pkrvars.hcl) | `images/ubuntu-24.04-base.img` |
| `ubuntu24-tool` | server | [vars/ubuntu24-tool.pkrvars.hcl](vars/ubuntu24-tool.pkrvars.hcl) | `images/ubuntu-24.04-tool.img` |
| `ubuntu24-desktop` | desktop | [vars/ubuntu24-desktop.pkrvars.hcl](vars/ubuntu24-desktop.pkrvars.hcl) | `images/ubuntu-24.04-desktop.img` |
| `rocky10-base` | server | [vars/rocky10-base.pkrvars.hcl](vars/rocky10-base.pkrvars.hcl) | `images/rocky-10-base.img` |
| `rocky10-tool` | server | [vars/rocky10-tool.pkrvars.hcl](vars/rocky10-tool.pkrvars.hcl) | `images/rocky-10-tool.img` |
| `rocky9-base` | server | [vars/rocky9-base.pkrvars.hcl](vars/rocky9-base.pkrvars.hcl) | `images/rocky-9-base.img` |
| `rocky9-tool` | server | [vars/rocky9-tool.pkrvars.hcl](vars/rocky9-tool.pkrvars.hcl) | `images/rocky-9-tool.img` |
| `rocky9-desktop` | desktop | [vars/rocky9-desktop.pkrvars.hcl](vars/rocky9-desktop.pkrvars.hcl) | `images/rocky-9-desktop.img` |
| `debian13-base` | server | [vars/debian13-base.pkrvars.hcl](vars/debian13-base.pkrvars.hcl) | `images/debian-13-base.img` |

Debian has no `tool` target: the toolchain's HashiCorp step resolves an apt
suite from `VERSION_CODENAME` and `releases.hashicorp.com` publishes no `trixie`
suite. Rocky 10 has no `desktop` target because nothing consumes one.

### Rocky base images need CRB before provisioning

`base` means the guest agent and the timezone, with no repository setup, so the
Rocky `base` targets do not run `scripts/rocky/epel.sh`. EPEL and CRB stay a
`tool`/`desktop` concern.

The consequence is that `scripts/provision.sh` cannot install the toolchain on a
Rocky `base` host as-is. The vendored installer runs `dnf install epel-release`
but never enables CRB, and EPEL packages — `mosh` among the toolchain baseline —
resolve their dependencies out of CRB. Enable it on the host first:

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

`machine_profile` is written to `/etc/provisioning/machine-profile.local` and
passed to shared installers as `TOOL_MACHINE_PROFILE`. Both `base` and `tool`
use `server`; a later `scripts/provision.sh` run installs any missing server
tools on a `base` image.

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
Tool declarations use concrete versions and `mise install --locked` consumes
the committed Linux artifact URLs and checksums without live release resolution.

## Upstream Image Imports

Some upstream images require normalization but no Packer build.

FreeBSD publishes `.qcow2.xz`, which `proxmox_download_file` cannot decompress.
Import it into the standard image/checksum contract:

```bash
./import-upstream.sh freebsd151
```

This command:

1. downloads `FreeBSD-15.1-RELEASE-amd64-BASIC-CLOUDINIT-ufs.qcow2.xz`;
2. verifies the official upstream archive sha256;
3. decompresses it to `images/freebsd-15.1-cloudinit-ufs.img`;
4. writes `images/freebsd-15.1-cloudinit-ufs.img.sha256` for the decompressed
   object that Proxmox will download.

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
for unattended builds.

Upstream imports are handled separately by `import-upstream.sh`; they are not
included in `build.sh all`.

The script confirms overwrite, clears the target's temporary output, runs
Packer, converts to compressed qcow2, and writes the image plus `.sha256` under
`images/`. The `output-*/` directory holding the raw qcow2 is removed as soon as
the sparsified image is in place, so `all` keeps only one build's intermediate
on disk at a time. Distribution and deployment are covered by Quick Start
steps 3–4.

## Dependency Management

Root `renovate.json` manages tracked dependency versions.

Images install prerequisites and the CLI toolchain through the same
`scripts/install` wrappers as `scripts/provision.sh`, driven by
[scripts/common/toolchain.sh](scripts/common/toolchain.sh). Tool pins and their
mise lockfile are maintained in `takanao14/dotfiles`; refresh the vendored copies with
`../scripts/install/vendor/sync.sh`.

**Not tracked (always installed as latest):**
- Unpinned APT/DNF packages installed by the shared package installer or Packer
  scripts (terraform, packer, vault, Firefox, VS Code, Wireshark, Podman, etc.)

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
