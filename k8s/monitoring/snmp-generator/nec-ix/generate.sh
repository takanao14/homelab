#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/../../../.." && pwd)
output="$repo_root/k8s/monitoring/charts/snmp-exporter/files/nec-ix.yml"
mib_url=https://jpn.nec.com/univerge/ix/Manual/MIB/PICO-SMI-MIB.txt
mib_sha256=049313b57a4ebefbda2815cccb7f2a1254d729ae29a9032ce1cd9777f72e2090
generator=${SNMP_GENERATOR:-generator}
mib_dirs=${MIB_DIRS:-/usr/share/snmp/mibs}
tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT

if [ -n "${PICO_SMI_MIB_PATH:-}" ]; then
  cp "$PICO_SMI_MIB_PATH" "$tmp_dir/PICO-SMI-MIB.txt"
else
  curl -fsSL --retry 3 -o "$tmp_dir/PICO-SMI-MIB.txt" "$mib_url"
fi

printf '%s  %s\n' "$mib_sha256" "$tmp_dir/PICO-SMI-MIB.txt" | shasum -a 256 -c -

# PICO-SMI imports ISDN-MIB for an unrelated subtree. Some Net-SNMP packages
# omit that MIB, so tolerate parser warnings while generating the selected NAPT subtree.
"$generator" --no-fail-on-parse-errors \
  -m "$tmp_dir" \
  -m "$mib_dirs" \
  generate \
  -g "$script_dir/generator.yml" \
  -o "$output"
