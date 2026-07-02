#!/usr/bin/env bash
#
# Run a Terragrunt command across every Proxmox node in this directory.
#
# Each node has its own `.envrc` that injects node-specific Proxmox
# credentials via sops. `terragrunt run-all` does not trigger direnv, so we
# use `direnv exec <dir>` to load each node's environment in isolation.
#
# tofu/terraform flags are passed via `terragrunt run -- <command> <flags>`,
# the explicit form that guarantees flags reach the underlying binary. With
# the shortcut form (`terragrunt apply -parallelism=1`) Terragrunt 1.0 parses
# `-parallelism` itself and never forwards it, so uploads run at the default
# parallelism of 10 (looks parallel) and exhaust RAM.
#
# Behavior:
#   - `apply` is auto-approved (-auto-approve).
#   - plan/apply/destroy/refresh pin terraform parallelism to PARALLELISM
#     (default 1), because each image is expanded into memory during upload
#     and parallel uploads can exhaust RAM. Override with PARALLELISM=N.
#   - Nodes run serially by default; set PARALLEL=1 to run them in parallel.
#
# Usage:
#   ./run-all.sh plan
#   ./run-all.sh apply
#   PARALLELISM=2 ./run-all.sh apply
#   PARALLEL=1 ./run-all.sh apply
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PARALLELISM="${PARALLELISM:-1}"

# Discover node stacks: every direct subdirectory holding a terragrunt.hcl.
# The script is symlinked into customimage/, so discovery follows the symlink's
# own directory and picks up that stack's nodes.
NODES=()
for dir in "${SCRIPT_DIR}"/*/; do
  [[ -f "${dir}terragrunt.hcl" ]] && NODES+=("$(basename "${dir}")")
done
if [[ ${#NODES[@]} -eq 0 ]]; then
  echo "Error: no node directories with terragrunt.hcl under ${SCRIPT_DIR}" >&2
  exit 1
fi

if [[ $# -eq 0 ]]; then
  echo "Usage: $0 <terragrunt-command> [args...]" >&2
  exit 1
fi

# Build the tofu/terraform argument list: <command> <user args> <injected flags>.
tf_cmd="$1"
shift
tofu_args=("${tf_cmd}" "$@")
case "${tf_cmd}" in
apply)
  tofu_args+=(-auto-approve)
  ;;
esac
case "${tf_cmd}" in
plan | apply | destroy | refresh)
  tofu_args+=(-parallelism="${PARALLELISM}")
  ;;
esac

run_node() {
  local node="$1"
  local dir="${SCRIPT_DIR}/${node}"
  echo "=== ${node} ==="
  # `direnv exec` loads the .envrc but does not change cwd, so cd into the
  # node directory first to let terragrunt find its terragrunt.hcl.
  (cd "${dir}" && direnv exec "${dir}" terragrunt run -- "${tofu_args[@]}")
}

if [[ "${PARALLEL:-0}" == "1" ]]; then
  pids=()
  for node in "${NODES[@]}"; do
    run_node "${node}" &
    pids+=($!)
  done
  status=0
  for pid in "${pids[@]}"; do
    wait "${pid}" || status=1
  done
  exit "${status}"
else
  for node in "${NODES[@]}"; do
    run_node "${node}"
  done
fi
