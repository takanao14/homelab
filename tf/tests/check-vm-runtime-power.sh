#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "$0")/../.." && pwd)
test_dir=$(mktemp -d "${TMPDIR:-/tmp}/homelab-vm-power.XXXXXX")
trap 'rm -rf "$test_dir"' EXIT

# Test a backend-free copy; never initialize a Terragrunt stack or read state.
cp "$repo_root"/tf/modules/proxmox-vm/*.tf "$test_dir/"
cp "$repo_root/tf/modules/proxmox-vm/runtime-power.tf.tftpl" "$test_dir/"
mkdir -p "$test_dir/modules/cloudinit-snippets"
cp "$repo_root"/tf/modules/proxmox-vm/modules/cloudinit-snippets/*.tf \
  "$test_dir/modules/cloudinit-snippets/"
cp -R "$repo_root/tf/modules/proxmox-vm/tests" "$test_dir/"
cp "$repo_root/tf/provider.tf" "$test_dir/provider.tf"
terraform -chdir="$test_dir" init -backend=false -input=false -no-color
terraform -chdir="$test_dir" test -no-color

# Terragrunt generates this override only in imagebuilder1's module cache.
cp "$test_dir/runtime-power.tf.tftpl" "$test_dir/runtime_power_override.tf"
terraform -chdir="$test_dir" test -test-directory=tests/runtime-power -no-color
