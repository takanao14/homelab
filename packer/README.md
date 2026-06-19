# Packer — Custom Cloud Images for Proxmox VE

Packer templates that build cloud-init enabled custom VM images for the Proxmox
VE homelab.

The build and the Proxmox registration are **decoupled** through S3 (SeaweedFS):

1. `build.sh` builds the image into `images/` and writes a `.sha256` sidecar.
2. `push.sh` uploads the image and its checksum to the SeaweedFS `cloud-images`
   bucket (`https://s3.home.butaco.net/cloud-images/`).
3. The Terragrunt stack in [`../tf/customimage`](../tf/customimage) makes each
   Proxmox node **download** the image from that URL (`proxmox_download_file`),
   pinned by the published sha256.

This means the host running `terragrunt apply` no longer needs the image files
locally — only network access to the bucket. Packer (which needs KVM/libguestfs)
can run on a dedicated builder, separate from where images are registered.

## Project Overview

- **Purpose**: Automated creation of cloud-init enabled golden images
- **Target Platform**: Proxmox VE
- **Supported OS**: Ubuntu 24.04, Rocky Linux 9/10, Debian 13
- **Image Variants**: Base (minimal) and XRDP (desktop environment with remote access)

## Requirements

### Build Requirements
- Packer >= 1.15.0
- QEMU tools (`qemu-img`)
- Proxmox VE API access
- Internet access for downloading base images and packages

### Upload (push to S3)

- `rclone`
- SeaweedFS S3 credentials with write access to the `cloud-images` bucket,
  exported as `SEAWEEDFS_S3_ENDPOINT` / `SEAWEEDFS_S3_ACCESS_KEY` /
  `SEAWEEDFS_S3_SECRET_KEY` (inject via `.envrc` / sops — never hardcode).

### Deployment

Proxmox download of the pushed images is handled by the Terragrunt stack in
[`../tf/customimage`](../tf/customimage) (module `tf/modules/proxmox-cloudimage`,
shared with the stock-image stack `tf/cloudimage`). See that directory for the
per-environment (`dev`/`prd`/`node2`/`node3`) configs.

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
├── build.sh            # Main build script
├── push.sh             # Upload built images + checksums to SeaweedFS S3
└── *.pkr.hcl           # Packer template files
```

## Quick Start

### 1. Set Environment Variables

```bash
# Required: Set the default user password for cloud-init
export PKR_VAR_user_password='your_secure_password'

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
```

### 3. Push Images to S3

```bash
# Upload one target (image + .sha256) to the cloud-images bucket
./push.sh ubuntu24

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

## Available Packer Templates

| Template | Description | Output |
|----------|-------------|--------|
| [ubuntu-24.04-custom.pkr.hcl](ubuntu-24.04-custom.pkr.hcl) | Ubuntu 24.04 base with QEMU Guest Agent | `images/ubuntu-24.04-custom.img` |
| [ubuntu-24.04-xrdp.pkr.hcl](ubuntu-24.04-xrdp.pkr.hcl) | Ubuntu 24.04 with XRDP + XFCE4 desktop | `images/ubuntu-24.04-xrdp.img` |
| [rocky-10-custom.pkr.hcl](rocky-10-custom.pkr.hcl) | Rocky Linux 10 base image | `images/rocky-10-custom.img` |
| [rocky-9-xrdp.pkr.hcl](rocky-9-xrdp.pkr.hcl) | Rocky Linux 9 with XRDP + XFCE desktop | `images/rocky-9-xrdp.img` |
| [debian-13-custom.pkr.hcl](debian-13-custom.pkr.hcl) | Debian 13 base image | `images/debian-13-custom.img` |

## Build Script Options

The `build.sh` script simplifies the build process:

```bash
./build.sh [OPTIONS] <IMAGE_TYPE>
```

**Options:**
- `-y` - Force overwrite existing images without prompting

**Available IMAGE_TYPE values:**
- `ubuntu24` - Ubuntu 24.04 base image
- `ubuntu24-xrdp` - Ubuntu 24.04 with XRDP
- `rocky10` - Rocky Linux 10 base image
- `rocky9-xrdp` - Rocky Linux 9 with XRDP
- `debian13` - Debian 13 base image

### Build Process

1. Checks if the output image already exists and prompts for confirmation
2. Removes the corresponding `output-*` directory if it exists
3. Runs Packer build with appropriate variables
4. Converts the output to compressed qcow2 format in the `images/` directory

### Build Output

**Intermediate files (temporary):**
- `output-ubuntu24-custom/`
- `output-ubuntu24-xrdp/`
- `output-rocky-10-custom/`
- `output-rocky-9-xrdp/`
- `output-debian-13-custom/`

**Final images (each with a `.sha256` sidecar):**
- `images/ubuntu-24.04-custom.img`
- `images/ubuntu-24.04-xrdp.img`
- `images/rocky-10-custom.img`
- `images/rocky-9-xrdp.img`
- `images/debian-13-custom.img`

## Image Distribution with S3 + Terragrunt

After building, push images to the SeaweedFS `cloud-images` bucket and then make
Proxmox download them via the Terragrunt stack in
[`../tf/customimage`](../tf/customimage):

```bash
# 1. Push built images + checksums to S3
./push.sh all

# 2. Deploy to production
cd ../tf/customimage/prd
terragrunt apply

# 3. Deploy to development
cd ../tf/customimage/dev
terragrunt apply
```

Each environment directory (`dev`/`prd`/`node2`/`node3`) holds a
`terragrunt.hcl` selecting which images to deploy and the target Proxmox node.
The shared module `tf/modules/proxmox-cloudimage` issues the download
(`proxmox_download_file`) from `https://s3.home.butaco.net/cloud-images/<file>`,
pinned to the sha256 published next to each object so rebuilt images are
re-downloaded. Image definitions (and the bucket base URL) are centralized in
`tf/customimage/images.hcl`.

## Dependency Management

This repository uses [Renovate](https://docs.renovatebot.com/) to automatically
track and update dependency versions, configured in the root `renovate.json`.

The XRDP images first install system-package prerequisites via
`../scripts/install/packages.sh global`, then bake the CLI toolchain (kubectl,
helm, terragrunt, opentofu, k9s, …) system-wide via
`../scripts/install/tools.sh global`. These wrappers are the single source of
truth shared with `scripts/provision.sh`; pinned tool versions are
Renovate-managed in the `takanao14/dotfiles` installers, not here.

The wrappers run the **vendored** installer copies in
`../scripts/install/vendor/` (uploaded to the guest by a `file` provisioner and
selected via `VENDOR_DIR`), so the build does not fetch them from GitHub at
runtime. Refresh those copies with `../scripts/install/vendor/sync.sh`.

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

## Features

### Base Images
- ✅ Cloud-init enabled
- ✅ QEMU Guest Agent installed
- ✅ Minimal package set
- ✅ SSH key authentication only (password auth disabled)
- ✅ Optimized for cloning

### XRDP Images
All base features plus:
- ✅ XFCE4 desktop environment
- ✅ XRDP remote desktop server
- ✅ Pre-configured for remote access

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
The build script will prompt you to confirm overwriting. Answer 'y' to proceed, or use `-y` flag to skip the prompt.

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

## License

MIT License. See the [repository root LICENSE](../LICENSE).
