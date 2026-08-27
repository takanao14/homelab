#!/bin/bash
set -euo pipefail

# Upload images and digests to SeaweedFS for tf/customimage downloads.
# Inject authentication through .envrc/SOPS:
#   SEAWEEDFS_S3_ENDPOINT     e.g. https://s3.home.butaco.net
#   SEAWEEDFS_S3_ACCESS_KEY   identity with non-Admin Read/Write/List actions
#   SEAWEEDFS_S3_SECRET_KEY
# Requires: rclone.

BUCKET="${SEAWEEDFS_CLOUD_IMAGES_BUCKET:-cloud-images}"
IMAGES_DIR="$(cd "$(dirname "$0")" && pwd)/images"

# Map targets to image basenames; CI checks parity with producers and tf/customimage.
# A case statement supports macOS' older Bash.
target_image() {
    case "$1" in
        ubuntu24-base) echo "ubuntu-24.04-base.img" ;;
        ubuntu24-tool) echo "ubuntu-24.04-tool.img" ;;
        ubuntu24-desktop) echo "ubuntu-24.04-desktop.img" ;;
        rocky10-base) echo "rocky-10-base.img" ;;
        rocky10-tool) echo "rocky-10-tool.img" ;;
        rocky9-base) echo "rocky-9-base.img" ;;
        rocky9-tool) echo "rocky-9-tool.img" ;;
        rocky9-desktop) echo "rocky-9-desktop.img" ;;
        debian13-base) echo "debian-13-base.img" ;;
        freebsd151) echo "freebsd-15.1-cloudinit-ufs.img" ;;
        *) return 1 ;;
    esac
}

list_targets() {
    cat << EOF
    debian13-base     debian-13-base.img
    freebsd151        freebsd-15.1-cloudinit-ufs.img
    rocky10-base      rocky-10-base.img
    rocky10-tool      rocky-10-tool.img
    rocky9-base       rocky-9-base.img
    rocky9-desktop    rocky-9-desktop.img
    rocky9-tool       rocky-9-tool.img
    ubuntu24-base     ubuntu-24.04-base.img
    ubuntu24-desktop  ubuntu-24.04-desktop.img
    ubuntu24-tool     ubuntu-24.04-tool.img
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
    $0 ubuntu24-base
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

# Configure rclone through environment variables to hide secrets from arguments.
setup_rclone() {
    export RCLONE_CONFIG_SEAWEEDFS_TYPE=s3
    export RCLONE_CONFIG_SEAWEEDFS_PROVIDER=Other
    export RCLONE_CONFIG_SEAWEEDFS_ACCESS_KEY_ID="$SEAWEEDFS_S3_ACCESS_KEY"
    export RCLONE_CONFIG_SEAWEEDFS_SECRET_ACCESS_KEY="$SEAWEEDFS_S3_SECRET_KEY"
    export RCLONE_CONFIG_SEAWEEDFS_ENDPOINT="$SEAWEEDFS_S3_ENDPOINT"
    export RCLONE_CONFIG_SEAWEEDFS_REGION=us-east-1
    # The scoped identity cannot create buckets; require the admin-created bucket.
    export RCLONE_CONFIG_SEAWEEDFS_NO_CHECK_BUCKET=true
    # Force PutObject; the scoped identity cannot start multipart uploads.
    export RCLONE_CONFIG_SEAWEEDFS_USE_MULTIPART_UPLOADS=false
}

run_rclone_copyto() {
    if ! rclone copyto --s3-use-multipart-uploads=false "$@"; then
        cat >&2 << EOF

Upload failed. If the error above is S3 403 AccessDenied, update the
SeaweedFS S3 identity and managed buckets on the server:

  cd ../ansible
  ansible-playbook playbooks/seaweedfs.yaml --tags seaweedfs

EOF
        exit 1
    fi
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
    run_rclone_copyto "$image_file" "seaweedfs:${BUCKET}/${image_name}"
    run_rclone_copyto "$checksum_file" "seaweedfs:${BUCKET}/${image_name}.sha256"
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
