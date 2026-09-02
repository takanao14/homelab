#!/bin/bash
set -euo pipefail

# Print help and exit with the specified status.
usage() {
    local exit_status="${1:-1}"
    cat << EOF
Usage: $0 [OPTION]

Build VM images using Packer

Every target is named <distro><version>-<role>. The role suffix says how much
is baked into the image, and each role contains the one before it:

    base     QEMU Guest Agent and the timezone (JST) only
    tool     base plus the shared CLI toolchain in /usr/local
    desktop  tool plus XFCE, XRDP, the Japanese IME and the GUI applications

OPTIONS:
    -y                Force overwrite existing images without prompting

TARGETS:
    ubuntu24-base     Ubuntu 24.04, agent and timezone only
    ubuntu24-tool     Ubuntu 24.04 with the shared CLI toolchain
    ubuntu24-desktop  Ubuntu 24.04 with XFCE/XRDP on top of the toolchain
    ubuntu26-base     Ubuntu 26.04, agent and timezone only
    ubuntu26-tool     Ubuntu 26.04 with the shared CLI toolchain
    ubuntu26-desktop  Ubuntu 26.04 with XFCE/XRDP on top of the toolchain
    rocky10-base      Rocky 10, agent and timezone only
    rocky10-tool      Rocky 10 with the shared CLI toolchain
    rocky9-base       Rocky 9, agent and timezone only
    rocky9-tool       Rocky 9 with the shared CLI toolchain
    rocky9-desktop    Rocky 9 with XFCE/XRDP on top of the toolchain
    debian13-base     Debian 13, agent and timezone only
    all               Build every target listed above, in order
    help              Display this help message

Debian has no tool target: the toolchain's HashiCorp step resolves an apt suite
from VERSION_CODENAME and releases.hashicorp.com publishes no trixie suite.

EXAMPLES:
    $0 ubuntu24-base
    $0 ubuntu24-desktop
    $0 -y all

EOF
    exit "$exit_status"
}

# Confirm overwrite when output already exists.
check_overwrite() {
    local image_file="$1"
    local output_dir="$2"
    if [ -f "$image_file" ] || [ -d "$output_dir" ]; then
        echo "Warning: Destination file '$image_file' or output directory '$output_dir' already exists"
        if [ "$FORCE_OVERWRITE" = false ]; then
            if [ ! -t 0 ]; then
                echo "Error: Non-interactive terminal and destination already exists. Use -y to force overwrite."
                exit 1
            fi
            read -p "Do you want to overwrite it? (y/N) " -n 1 -r
            echo
            if [[ ! $REPLY =~ ^[Yy]$ ]]; then
                echo "Build cancelled by user"
                exit 0
            fi
        fi
        # Unconditional: rm -f/-rf tolerate a missing path, and a guarding
        # test that fails here would be the function's exit status and trip
        # set -e (only one of the two usually exists).
        rm -f "$image_file"
        rm -rf "$output_dir"
    fi
}

# Run a Packer build for the given target.
# Arguments: var_file, packer_output, image_file
build_image() {
    local var_file="$1"
    local packer_output="$2"
    local image_file="$3"

    local packer_output_dir packer_vm_name
    packer_output_dir=$(dirname "$packer_output")
    packer_vm_name=$(basename "$packer_output")

    echo "Setting read permissions on host kernel for libguestfs..."
    sudo chmod 0644 /boot/vmlinuz-*

    check_overwrite "$image_file" "$packer_output_dir"

    echo "Initializing Packer..."
    packer init "$PACKER_TEMPLATE"

    echo "Building ${packer_vm_name}..."
    packer build \
        -var-file "$var_file" \
        -var "output_directory=${packer_output_dir}" \
        -var "vm_name=${packer_vm_name}" \
        -var "image_name=${image_file}" \
        "$PACKER_TEMPLATE"

    if [ ! -f "${packer_output}" ]; then
        echo "Error: Source file '${packer_output}' not found after build"
        exit 1
    fi

    if [ ! -f "${image_file}" ]; then
        echo "Error: Destination file '${image_file}' not found after build"
        exit 1
    fi

    # The sparsified image is the only artifact worth keeping. Drop the raw
    # qcow2 now rather than at the next build, so `all` does not carry every
    # intermediate to the end of the run.
    echo "Removing intermediate build output '${packer_output_dir}'..."
    rm -rf "${packer_output_dir}"

    # Recompute the sidecar digest so Terraform detects rebuilt images.
    echo "Writing checksum for ${image_file}..."
    sha256sum "${image_file}" | cut -d' ' -f1 > "${image_file}.sha256"
}

# Every target shares one template; vars/<target>.pkrvars.hcl carries the
# distro, machine profile and provisioner list.
readonly PACKER_TEMPLATE="image.pkr.hcl"

# Map a target to var file/output; CI checks parity with push.sh and tf/customimage.
build_target() {
    case "$1" in
        ubuntu24-base)
            build_image \
                "vars/ubuntu24-base.pkrvars.hcl" \
                "output-ubuntu-24.04-base/ubuntu-24.04-base.qcow2" \
                "images/ubuntu-24.04-base.img"
            ;;
        ubuntu24-tool)
            build_image \
                "vars/ubuntu24-tool.pkrvars.hcl" \
                "output-ubuntu-24.04-tool/ubuntu-24.04-tool.qcow2" \
                "images/ubuntu-24.04-tool.img"
            ;;
        ubuntu24-desktop)
            build_image \
                "vars/ubuntu24-desktop.pkrvars.hcl" \
                "output-ubuntu-24.04-desktop/ubuntu-24.04-desktop.qcow2" \
                "images/ubuntu-24.04-desktop.img"
            ;;
        ubuntu26-base)
            build_image \
                "vars/ubuntu26-base.pkrvars.hcl" \
                "output-ubuntu-26.04-base/ubuntu-26.04-base.qcow2" \
                "images/ubuntu-26.04-base.img"
            ;;
        ubuntu26-tool)
            build_image \
                "vars/ubuntu26-tool.pkrvars.hcl" \
                "output-ubuntu-26.04-tool/ubuntu-24.06-tool.qcow2" \
                "images/ubuntu-26.04-tool.img"
            ;;
        ubuntu26-desktop)
            build_image \
                "vars/ubuntu26-desktop.pkrvars.hcl" \
                "output-ubuntu-26.04-desktop/ubuntu-26.04-desktop.qcow2" \
                "images/ubuntu-26.04-desktop.img"
            ;;
        rocky10-base)
            build_image \
                "vars/rocky10-base.pkrvars.hcl" \
                "output-rocky-10-base/rocky-10-base.qcow2" \
                "images/rocky-10-base.img"
            ;;
        rocky10-tool)
            build_image \
                "vars/rocky10-tool.pkrvars.hcl" \
                "output-rocky-10-tool/rocky-10-tool.qcow2" \
                "images/rocky-10-tool.img"
            ;;
        rocky9-base)
            build_image \
                "vars/rocky9-base.pkrvars.hcl" \
                "output-rocky-9-base/rocky-9-base.qcow2" \
                "images/rocky-9-base.img"
            ;;
        rocky9-tool)
            build_image \
                "vars/rocky9-tool.pkrvars.hcl" \
                "output-rocky-9-tool/rocky-9-tool.qcow2" \
                "images/rocky-9-tool.img"
            ;;
        rocky9-desktop)
            build_image \
                "vars/rocky9-desktop.pkrvars.hcl" \
                "output-rocky-9-desktop/rocky-9-desktop.qcow2" \
                "images/rocky-9-desktop.img"
            ;;
        debian13-base)
            build_image \
                "vars/debian13-base.pkrvars.hcl" \
                "output-debian-13-base/debian-13-base.qcow2" \
                "images/debian-13-base.img"
            ;;
        *)
            echo "Error: Unknown build target '$1'"
            usage
            ;;
    esac
}

# All targets, in build order: the cheap base images first, so an interrupted
# `all` still leaves the quickest-to-rebuild targets done.
ALL_TARGETS=(
    ubuntu24-base rocky10-base rocky9-base debian13-base ubuntu26-base
    ubuntu24-tool rocky10-tool rocky9-tool ubuntu26-tool
    ubuntu24-desktop rocky9-desktop ubuntu26-desktop
)

FORCE_OVERWRITE=false
while [[ $# -gt 0 ]]; do
    case "$1" in
        -y)
            FORCE_OVERWRITE=true
            shift
            ;;
        -h|--help)
            usage 0
            ;;
        -*)
            echo "Error: Unknown option '$1'"
            usage
            ;;
        *)
            break
            ;;
    esac
done

if [ $# -eq 0 ]; then
    echo "Error: No build target specified"
    usage
fi

BUILD_TARGET="$1"

mkdir -p images

case "$BUILD_TARGET" in
    help|--help|-h)
        usage 0
        ;;
    all)
        for target in "${ALL_TARGETS[@]}"; do
            build_target "$target"
        done
        ;;
    *)
        build_target "$BUILD_TARGET"
        ;;
esac

echo "Build completed successfully!"
