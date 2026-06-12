#!/bin/bash
set -euo pipefail

# Print help and exit with the specified status.
usage() {
    local exit_status="${1:-1}"
    cat << EOF
Usage: $0 [OPTION]

Build VM images using Packer

OPTIONS:
    -y             Force overwrite existing images without prompting
    ubuntu24       Build a basic Ubuntu 24.04 image with the QEMU Guest Agent and the timezone set to JST
    ubuntu24-xrdp  Build Ubuntu 24.04 image with XRDP service
    rocky10        Build a basic Rocky 10 Linux image with the timezone set to JST
    rocky9         Build a basic Rocky 9 Linux image with the timezone set to JST
    rocky9-xrdp    Build Rocky 9 Linux image with XRDP service
    debian13       Build a basic Debian 13 image
    help           Display this help message

EXAMPLES:
    $0 ubuntu24
    $0 ubuntu24-xrdp

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
        [ -f "$image_file" ] && rm -f "$image_file"
        [ -d "$output_dir" ] && rm -rf "$output_dir"
    fi
}

# Run a Packer build for the given target.
# Arguments: packer_file, packer_output, image_file
build_image() {
    local packer_file="$1"
    local packer_output="$2"
    local image_file="$3"

    local packer_output_dir=$(dirname "$packer_output")
    local packer_vm_name=$(basename "$packer_output")

    echo "Setting read permissions on host kernel for libguestfs..."
    sudo chmod 0644 /boot/vmlinuz-*

    check_overwrite "$image_file" "$packer_output_dir"

    echo "Initializing Packer..."
    packer init "$packer_file"

    echo "Building ${packer_vm_name}..."
    packer build \
        -var "output_directory=${packer_output_dir}" \
        -var "vm_name=${packer_vm_name}" \
        -var "image_name=${image_file}" \
        "$packer_file"

    if [ ! -f "${packer_output}" ]; then
        echo "Error: Source file '${packer_output}' not found after build"
        exit 1
    fi

    if [ ! -f "${image_file}" ]; then
        echo "Error: Destination file '${image_file}' not found after build"
        exit 1
    fi
}

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

# Map CLI targets to their Packer templates and outputs.
case "$BUILD_TARGET" in
    ubuntu24)
        build_image \
            "ubuntu-24.04-custom.pkr.hcl" \
            "output-ubuntu24-custom/ubuntu-24.04-custom.qcow2" \
            "images/ubuntu-24.04-custom.img"
        ;;
    ubuntu24-xrdp)
        build_image \
            "ubuntu-24.04-xrdp.pkr.hcl" \
            "output-ubuntu24-xrdp/ubuntu-24.04-xrdp.qcow2" \
            "images/ubuntu-24.04-xrdp.img"
        ;;
    rocky10)
        build_image \
            "rocky-10-custom.pkr.hcl" \
            "output-rocky-10-custom/rocky-10-custom.qcow2" \
            "images/rocky-10-custom.img"
        ;;
    rocky9)
        build_image \
            "rocky-9-custom.pkr.hcl" \
            "output-rocky-9-custom/rocky-9-custom.qcow2" \
            "images/rocky-9-custom.img"
        ;;
    rocky9-xrdp)
        build_image \
            "rocky-9-xrdp.pkr.hcl" \
            "output-rocky-9-xrdp/rocky-9-xrdp.qcow2" \
            "images/rocky-9-xrdp.img"
        ;;
    debian13)
        build_image \
            "debian-13-custom.pkr.hcl" \
            "output-debian-13-custom/debian-13-custom.qcow2" \
            "images/debian-13-custom.img"
        ;;
    help|--help|-h)
        usage 0
        ;;
    *)
        echo "Error: Unknown build target '$BUILD_TARGET'"
        usage
        ;;
esac

echo "Build completed successfully!"
