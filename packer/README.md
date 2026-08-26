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

### Deployment

[`../tf/customimage`](../tf/customimage) downloads pushed images through the
shared `tf/modules/proxmox-cloudimage` module.

## Directory Structure

```
.
├── cinit/              # Cloud-init configuration templates for Packer
├── images/             # Generated image output directory (*.img files)
├── output-*/           # Packer build artifacts (temporary, gitignored)
├── scripts/
│   ├── ubuntu/         # Shell provisioners for Ubuntu
│   ├── rocky/          # Shell provisioners for Rocky Linux
│   └── debian/         # Shell provisioners for Debian
├── vars/               # Per-target inputs (URL, checksum, user, script list)
├── basic.pkr.hcl       # Shared template: headless server images
├── xrdp.pkr.hcl        # Shared template: XRDP desktop images
├── build.sh            # Main build script (selects template + var file)
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
# Build base Ubuntu 24.04 image with QEMU Guest Agent
./build.sh ubuntu24

# Build Ubuntu 24.04 with XRDP and XFCE desktop
./build.sh ubuntu24-xrdp

# Build base Rocky Linux 10 image
./build.sh rocky10

# Build Rocky Linux 9 with XRDP and XFCE desktop
./build.sh rocky9-xrdp

# Build base Debian 13 image
./build.sh debian13

# Import FreeBSD 15.1 upstream cloud-init image
./import-upstream.sh freebsd151
```

### 3. Push Images to S3

```bash
# Upload one target (image + .sha256) to the cloud-images bucket
./push.sh ubuntu24

# Upload imported FreeBSD 15.1 image
./push.sh freebsd151

# Or upload every image currently in images/
./push.sh all
```

### 4. Deploy Images to Proxmox

`terragrunt apply` makes each Proxmox node download the pushed images. The
checksum is read from the bucket at plan time, so an image must be pushed
(step 3) before it can be deployed.

```bash
cd ../tf/customimage/prd
terragrunt apply
```

## Available Build Targets

Targets use [basic.pkr.hcl](basic.pkr.hcl) or
[xrdp.pkr.hcl](xrdp.pkr.hcl). URLs, checksums, users, cloud-init paths and
provisioners live in `vars/<target>.pkrvars.hcl`.

| Target | Template | Var file | Output |
|--------|----------|----------|--------|
| `ubuntu24` | basic | [vars/ubuntu24.pkrvars.hcl](vars/ubuntu24.pkrvars.hcl) | `images/ubuntu-24.04-custom.img` |
| `ubuntu24-xrdp` | xrdp | [vars/ubuntu24-xrdp.pkrvars.hcl](vars/ubuntu24-xrdp.pkrvars.hcl) | `images/ubuntu-24.04-xrdp.img` |
| `rocky10` | basic | [vars/rocky10.pkrvars.hcl](vars/rocky10.pkrvars.hcl) | `images/rocky-10-custom.img` |
| `rocky9` | basic | [vars/rocky9.pkrvars.hcl](vars/rocky9.pkrvars.hcl) | `images/rocky-9-custom.img` |
| `rocky9-xrdp` | xrdp | [vars/rocky9-xrdp.pkrvars.hcl](vars/rocky9-xrdp.pkrvars.hcl) | `images/rocky-9-xrdp.img` |
| `debian13` | basic | [vars/debian13.pkrvars.hcl](vars/debian13.pkrvars.hcl) | `images/debian-13-custom.img` |

To add a target, create a `vars/<target>.pkrvars.hcl` and register the target in
`build.sh`, `push.sh` and `tf/customimage/images.hcl`
(`scripts/check-image-refs.sh` verifies the filename mapping in CI).

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
`images/`. Distribution and deployment are covered by Quick Start steps 3–4.

## Dependency Management

Root `renovate.json` manages tracked dependency versions.

XRDP images install prerequisites and the CLI toolchain through the same
`scripts/install` wrappers as `scripts/provision.sh`. Tool pins are maintained
in `takanao14/dotfiles`.

Packer writes the image role to `/etc/homelab/machine-profile`: XRDP images are
`desktop` and basic cloud images are `server`. The same
`TOOL_MACHINE_PROFILE=desktop|server` contract is passed to shared installers
and later consumed by `scripts/provision.sh`.

Builds use vendored installers from `../scripts/install/vendor/`; refresh them
with `../scripts/install/vendor/sync.sh`.

**Not tracked (always installed as latest):**
- Unpinned APT/DNF packages installed by the shared package installer or Packer
  scripts (terraform, packer, vault, Firefox, VS Code, Wireshark, Podman, etc.)

## Customization

### Manual Packer Build

Run Packer directly for custom configurations:

```bash
packer build \
  -var "output_directory=custom-output" \
  -var "vm_name=custom.qcow2" \
  -var "image_name=image/custom.img" \
  ubuntu-24.04-custom.pkr.hcl
```

### Modifying Provisioning Scripts

Edit scripts in the `scripts/` directory:
- `scripts/ubuntu/` - Ubuntu-specific provisioners
- `scripts/rocky/` - Rocky Linux-specific provisioners
- `scripts/debian/` - Debian-specific provisioners

All scripts should be:
- Idempotent
- Follow bash best practices (`set -euo pipefail`)

### Cloud-init Configuration

Modify templates in `cinit/` directory to customize:
- Network configuration
- SSH key injection
- Package installation
- User creation

## Security Considerations

- **SSH Authentication**: Password authentication is disabled; SSH key-only access
- **Default User**: Created via cloud-init with configurable password
- **Minimal Surface**: Only necessary packages are installed
- **Regular Updates**: Rebuild images regularly to include security patches
- **No Hardcoded Secrets**: All sensitive data passed via environment variables

## Troubleshooting

### Build Fails with "Permission Denied"
Ensure the Packer user has sudo access in the base cloud image.

### Image Already Exists
Confirm overwrite or pass `-y`.

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

## License

MIT License. See the [repository root LICENSE](../LICENSE).
