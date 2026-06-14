#!/bin/bash
set -euo pipefail

# Upload Packer-built images (and their .sha256 digests) to the SeaweedFS
# `cloud-images` bucket. Proxmox pulls them from there via tf/customimage
# (proxmox_download_file), decoupling the build host from the deploy host.
#
# Auth is taken from the environment (inject via .envrc / sops, never hardcode):
#   SEAWEEDFS_S3_ENDPOINT     e.g. https://s3.home.butaco.net
#   SEAWEEDFS_S3_ACCESS_KEY   identity with Write:cloud-images
#   SEAWEEDFS_S3_SECRET_KEY
#
# Requires: rclone.

BUCKET="${SEAWEEDFS_CLOUD_IMAGES_BUCKET:-cloud-images}"
IMAGES_DIR="$(cd "$(dirname "$0")" && pwd)/images"

# Map CLI targets to the image basename produced by build.sh. Keep in sync with
# the case block in build.sh.
declare -A TARGET_IMAGE=(
    [ubuntu24]="ubuntu-24.04-custom.img"
    [ubuntu24-xrdp]="ubuntu-24.04-xrdp.img"
    [rocky10]="rocky-10-custom.img"
    [rocky9]="rocky-9-custom.img"
    [rocky9-xrdp]="rocky-9-xrdp.img"
    [debian13]="debian-13-custom.img"
)

usage() {
    local exit_status="${1:-1}"
    cat << EOF
Usage: $0 <TARGET|all>

Upload built images and their checksums to the SeaweedFS cloud-images bucket.

TARGETS:
$(for t in "${!TARGET_IMAGE[@]}"; do printf '    %-14s %s\n' "$t" "${TARGET_IMAGE[$t]}"; done | sort)
    all            Upload every *.img present in images/

ENVIRONMENT:
    SEAWEEDFS_S3_ENDPOINT, SEAWEEDFS_S3_ACCESS_KEY, SEAWEEDFS_S3_SECRET_KEY

EXAMPLES:
    $0 ubuntu24
    $0 all
EOF
    exit "$exit_status"
}

# Validate required credentials are present.
require_env() {
    local missing=0
    for var in SEAWEEDFS_S3_ENDPOINT SEAWEEDFS_S3_ACCESS_KEY SEAWEEDFS_S3_SECRET_KEY; do
        if [ -z "${!var:-}" ]; then
            echo "Error: required environment variable '$var' is not set" >&2
            missing=1
        fi
    done
    [ "$missing" -eq 0 ] || exit 1

    if ! command -v rclone > /dev/null 2>&1; then
        echo "Error: rclone is not installed" >&2
        exit 1
    fi
}

# Configure an on-the-fly rclone remote via env vars so secrets never appear in
# the process arguments.
setup_rclone() {
    export RCLONE_CONFIG_SEAWEEDFS_TYPE=s3
    export RCLONE_CONFIG_SEAWEEDFS_PROVIDER=Other
    export RCLONE_CONFIG_SEAWEEDFS_ACCESS_KEY_ID="$SEAWEEDFS_S3_ACCESS_KEY"
    export RCLONE_CONFIG_SEAWEEDFS_SECRET_ACCESS_KEY="$SEAWEEDFS_S3_SECRET_KEY"
    export RCLONE_CONFIG_SEAWEEDFS_ENDPOINT="$SEAWEEDFS_S3_ENDPOINT"
    export RCLONE_CONFIG_SEAWEEDFS_REGION=us-east-1
}

# Upload one image and its sidecar checksum.
push_image() {
    local image_name="$1"
    local image_file="${IMAGES_DIR}/${image_name}"
    local checksum_file="${image_file}.sha256"

    if [ ! -f "$image_file" ]; then
        echo "Error: '$image_file' not found. Build it first with ./build.sh" >&2
        exit 1
    fi
    if [ ! -f "$checksum_file" ]; then
        echo "Error: '$checksum_file' not found. Rebuild with ./build.sh to generate it" >&2
        exit 1
    fi

    echo "Uploading ${image_name} -> seaweedfs:${BUCKET}/${image_name}"
    rclone copyto "$image_file" "seaweedfs:${BUCKET}/${image_name}"
    rclone copyto "$checksum_file" "seaweedfs:${BUCKET}/${image_name}.sha256"
}

[ $# -eq 1 ] || usage
case "$1" in
    -h | --help | help) usage 0 ;;
esac

require_env
setup_rclone

if [ "$1" = "all" ]; then
    shopt -s nullglob
    found=0
    for image_file in "${IMAGES_DIR}"/*.img; do
        push_image "$(basename "$image_file")"
        found=1
    done
    [ "$found" -eq 1 ] || { echo "Error: no images found in ${IMAGES_DIR}" >&2; exit 1; }
else
    image_name="${TARGET_IMAGE[$1]:-}"
    [ -n "$image_name" ] || { echo "Error: unknown target '$1'" >&2; usage; }
    push_image "$image_name"
fi

echo "Upload completed successfully!"
