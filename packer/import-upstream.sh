#!/bin/bash
set -euo pipefail

# Import upstream cloud images that do not need a full Packer build, but still
# need normalization before publishing to the SeaweedFS cloud-images bucket.
#
# The generated output follows the same contract as build.sh:
#   images/<name>.img
#   images/<name>.img.sha256
#
# Requires: curl, xz, sha256sum.

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
IMAGES_DIR="${SCRIPT_DIR}/images"
DOWNLOAD_DIR="${SCRIPT_DIR}/downloads"

FREEBSD_151_BASE_URL="https://download.freebsd.org/releases/VM-IMAGES/15.1-RELEASE/amd64/Latest"
FREEBSD_151_ARCHIVE="FreeBSD-15.1-RELEASE-amd64-BASIC-CLOUDINIT-ufs.qcow2.xz"
FREEBSD_151_SHA256="e4ca4db889f8559c9b9dfcacc70405c038476f4b6d41649b152d3809a2ed9e1f"
FREEBSD_151_OUTPUT="freebsd-15.1-cloudinit-ufs.img"

usage() {
    local exit_status="${1:-1}"
    cat << EOF
Usage: $0 [OPTION] <TARGET|all>

Import upstream cloud images into packer/images/.

OPTIONS:
    -y             Force overwrite existing images without prompting
    freebsd151     Import FreeBSD 15.1 BASIC-CLOUDINIT UFS qcow2 image
    all            Import every upstream target listed above
    help           Display this help message

EXAMPLES:
    $0 freebsd151
    $0 -y all
EOF
    exit "$exit_status"
}

require_tools() {
    local missing=0
    for tool in curl xz sha256sum; do
        if ! command -v "$tool" > /dev/null 2>&1; then
            echo "Error: required command '$tool' is not installed" >&2
            missing=1
        fi
    done
    [ "$missing" -eq 0 ] || exit 1
}

confirm_overwrite() {
    local output_file="$1"
    if [ ! -f "$output_file" ] && [ ! -f "${output_file}.sha256" ]; then
        return
    fi

    echo "Warning: Destination file '${output_file}' or checksum already exists"
    if [ "$FORCE_OVERWRITE" = false ]; then
        if [ ! -t 0 ]; then
            echo "Error: Non-interactive terminal and destination already exists. Use -y to force overwrite." >&2
            exit 1
        fi
        read -p "Do you want to overwrite it? (y/N) " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            echo "Import cancelled by user"
            exit 0
        fi
    fi

    rm -f "$output_file" "${output_file}.sha256"
}

download_archive() {
    local url="$1"
    local archive_file="$2"

    if [ -f "$archive_file" ]; then
        echo "Using cached archive ${archive_file}"
        return
    fi

    echo "Downloading ${url}"
    curl -fL --retry 3 --retry-delay 5 -o "$archive_file" "$url"
}

verify_sha256() {
    local file="$1"
    local expected_sha256="$2"

    echo "Verifying upstream checksum for ${file}"
    printf '%s  %s\n' "$expected_sha256" "$file" | sha256sum -c -
}

write_output_sha256() {
    local output_file="$1"

    echo "Writing checksum for ${output_file}"
    sha256sum "$output_file" | cut -d' ' -f1 > "${output_file}.sha256"
}

import_freebsd151() {
    local archive_file="${DOWNLOAD_DIR}/${FREEBSD_151_ARCHIVE}"
    local output_file="${IMAGES_DIR}/${FREEBSD_151_OUTPUT}"

    mkdir -p "$IMAGES_DIR" "$DOWNLOAD_DIR"
    confirm_overwrite "$output_file"
    download_archive "${FREEBSD_151_BASE_URL}/${FREEBSD_151_ARCHIVE}" "$archive_file"
    verify_sha256 "$archive_file" "$FREEBSD_151_SHA256"

    echo "Decompressing ${archive_file} -> ${output_file}"
    xz -dc "$archive_file" > "$output_file"
    write_output_sha256 "$output_file"
}

import_target() {
    case "$1" in
        freebsd151)
            import_freebsd151
            ;;
        *)
            echo "Error: Unknown import target '$1'" >&2
            usage
            ;;
    esac
}

ALL_TARGETS=(freebsd151)

FORCE_OVERWRITE=false
while [[ $# -gt 0 ]]; do
    case "$1" in
        -y)
            FORCE_OVERWRITE=true
            shift
            ;;
        -h | --help | help)
            usage 0
            ;;
        -*)
            echo "Error: Unknown option '$1'" >&2
            usage
            ;;
        *)
            break
            ;;
    esac
done

[ $# -eq 1 ] || usage

require_tools

if [ "$1" = "all" ]; then
    for target in "${ALL_TARGETS[@]}"; do
        import_target "$target"
    done
else
    import_target "$1"
fi

echo "Import completed successfully!"
