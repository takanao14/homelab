#!/usr/bin/env bash
set -euo pipefail

# Cross-check the hardcoded image-filename maps against the image definitions
# in tf/customimage/images.hcl (and tf/cloudimage/images.hcl).
#
# The target -> image-file mapping is duplicated across build.sh, push.sh and
# create-vm.sh, and a typo there only surfaces at VM-create time as a Proxmox
# "file not found" (this actually happened: create-vm.sh once pointed at
# debian13-custom.img while the real file is debian-13-custom.img). This script
# fails fast in CI instead (.github/workflows/image-refs.yaml).
#
# Checks:
#   1. create-vm.sh FILE_IDs        exist in customimage or cloudimage images.hcl
#   2. build.sh output image names  exist in customimage images.hcl
#   3. push.sh target image names   match customimage images.hcl exactly
#      (both directions: every pushable image is defined, every defined image
#      is pushable)
#
# Avoids bash-4-only features (mapfile, associative arrays) so it also runs
# on macOS' stock /bin/bash.

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

CUSTOMIMAGE_HCL="${REPO_ROOT}/tf/customimage/images.hcl"
CLOUDIMAGE_HCL="${REPO_ROOT}/tf/cloudimage/images.hcl"
CREATE_VM="${REPO_ROOT}/scripts/create-vm.sh"
BUILD_SH="${REPO_ROOT}/packer/build.sh"
PUSH_SH="${REPO_ROOT}/packer/push.sh"

# Extract file_name values from an images.hcl.
hcl_file_names() {
    grep -o 'file_name[[:space:]]*=[[:space:]]*"[^"]*"' "$1" | sed 's/.*"\(.*\)"/\1/' | sort -u
}

defined_custom="$(hcl_file_names "$CUSTOMIMAGE_HCL")"
defined_all="$(printf '%s\n%s\n' "$defined_custom" "$(hcl_file_names "$CLOUDIMAGE_HCL")" | sort -u)"

status=0

# check_subset <label> <defined-set> <items> <message>: every item must be in
# the set; <message> describes what a missing item means.
check_subset() {
    local label="$1" defined="$2" items="$3" message="$4" item
    while IFS= read -r item; do
        [ -n "$item" ] || continue
        if ! printf '%s\n' "$defined" | grep -qFx "$item"; then
            echo "ERROR: ${label}: '${item}' ${message}" >&2
            status=1
        fi
    done <<< "$items"
}

# 1. create-vm.sh: local:iso/<file> references (stock or custom images).
create_vm_files="$(grep -o 'local:iso/[^"]*' "$CREATE_VM" | sed 's|local:iso/||' | sort -u)"
check_subset "create-vm.sh" "$defined_all" "$create_vm_files" \
    "is not defined in images.hcl"

# 2. build.sh: images/<file> build outputs (custom images only).
build_files="$(grep -o '"images/[^"]*\.img"' "$BUILD_SH" | sed 's|"images/||; s|"$||' | sort -u)"
check_subset "build.sh" "$defined_custom" "$build_files" \
    "is not defined in tf/customimage/images.hcl"

# 3. push.sh: target_image() basenames, bidirectional against customimage.
push_files="$(sed -n 's/.*echo "\([^"]*\.img\)".*/\1/p' "$PUSH_SH" | sort -u)"
check_subset "push.sh" "$defined_custom" "$push_files" \
    "is not defined in tf/customimage/images.hcl"
check_subset "tf/customimage/images.hcl" "$push_files" "$defined_custom" \
    "has no push.sh target"

if [ "$status" -ne 0 ]; then
    echo "Image filename maps are out of sync. Fix the file(s) above to match tf/customimage/images.hcl." >&2
    exit 1
fi
echo "OK: image filename maps are consistent."
