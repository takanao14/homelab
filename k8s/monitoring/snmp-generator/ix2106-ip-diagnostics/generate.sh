#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/../../../.." && pwd)
output="$repo_root/k8s/monitoring/charts/snmp-exporter/files/ix2106-ip-diagnostics.yml"
generator=${SNMP_GENERATOR:-generator}
mib_dirs=${MIB_DIRS:-/usr/share/snmp/mibs}

"$generator" \
  -m "$mib_dirs" \
  generate \
  -g "$script_dir/generator.yml" \
  -o "$output"
