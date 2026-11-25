#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
test_root=$(mktemp -d)
trap 'rm -rf -- "$test_root"' EXIT

for module in proxmox-vm proxmox-container; do
  source_dir="$repo_root/tf/modules/$module"
  test_dir="$test_root/$module"
  mkdir "$test_dir"
  cp "$source_dir"/*.tf "$repo_root/tf/provider.tf" "$test_dir/"
  cp -R "$source_dir/tests" "$test_dir/"
  if [[ -d "$source_dir/modules" ]]; then
    cp -R "$source_dir/modules" "$test_dir/"
  fi

  tofu -chdir="$test_dir" init -backend=false -input=false
  tofu -chdir="$test_dir" test
done
