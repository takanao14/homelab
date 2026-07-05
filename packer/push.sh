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

# Map CLI targets to image basenames produced by build.sh or import-upstream.sh.
# Keep this in sync with those scripts and tf/customimage/images.hcl; CI
# verifies the mapping via scripts/check-image-refs.sh. Use a case statement
# instead of an associative array so the script works with macOS' older
# /bin/bash.
target_image() {
    case "$1" in
        ubuntu24) echo "ubuntu-24.04-custom.img" ;;
        ubuntu24-xrdp) echo "ubuntu-24.04-xrdp.img" ;;
        rocky10) echo "rocky-10-custom.img" ;;
        rocky9) echo "rocky-9-custom.img" ;;
        rocky9-xrdp) echo "rocky-9-xrdp.img" ;;
        debian13) echo "debian-13-custom.img" ;;
        freebsd151) echo "freebsd-15.1-cloudinit-ufs.img" ;;
        *) return 1 ;;
    esac
}

list_targets() {
    cat << EOF
    debian13       debian-13-custom.img
    freebsd151     freebsd-15.1-cloudinit-ufs.img
    rocky10        rocky-10-custom.img
    rocky9         rocky-9-custom.img
    rocky9-xrdp    rocky-9-xrdp.img
    ubuntu24       ubuntu-24.04-custom.img
    ubuntu24-xrdp  ubuntu-24.04-xrdp.img
EOF
}

usage() {
    local exit_status="${1:-1}"
    cat << EOF
Usage: $0 <TARGET|all>

Upload built images and their checksums to the SeaweedFS cloud-images bucket.

TARGETS:
$(list_targets)
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
    # The imagebuilder identity is scoped to cloud-images and cannot create
    # buckets. Without this, rclone attempts CreateBucket before upload and
    # fails with 403. The bucket must already exist (admin-created).
    export RCLONE_CONFIG_SEAWEEDFS_NO_CHECK_BUCKET=true
    # Keep uploads on plain PutObject. rclone's default S3 behavior switches
    # large files to multipart upload, but SeaweedFS rejects CreateMultipartUpload
    # for the scoped imagebuilder identity with AccessDenied.
    export RCLONE_CONFIG_SEAWEEDFS_USE_MULTIPART_UPLOADS=false
}

# Upload one image and its sidecar checksum.
push_image() {
    local image_name="$1"
    local image_file="${IMAGES_DIR}/${image_name}"
    local checksum_file="${image_file}.sha256"

    if [ ! -f "$image_file" ]; then
        echo "Error: '$image_file' not found. Build it with ./build.sh or import it with ./import-upstream.sh first" >&2
        exit 1
    fi
    if [ ! -f "$checksum_file" ]; then
        echo "Error: '$checksum_file' not found. Rebuild or re-import the image to generate it" >&2
        exit 1
    fi

    echo "Uploading ${image_name} -> seaweedfs:${BUCKET}/${image_name}"
    rclone copyto --s3-use-multipart-uploads=false "$image_file" "seaweedfs:${BUCKET}/${image_name}"
    rclone copyto --s3-use-multipart-uploads=false "$checksum_file" "seaweedfs:${BUCKET}/${image_name}.sha256"
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
    image_name="$(target_image "$1")" || { echo "Error: unknown target '$1'" >&2; usage; }
    push_image "$image_name"
fi

echo "Upload completed successfully!"
