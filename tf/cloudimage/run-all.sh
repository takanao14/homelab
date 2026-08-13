#!/usr/bin/env bash
#
# Run Terragrunt across nodes with each node's direnv credentials.
# `terragrunt run --` is required to forward Terraform flags on Terragrunt 1.0.
#
# Behavior:
#   - `apply` is auto-approved (-auto-approve).
#   - Image operations use PARALLELISM (default 1) to limit memory use.
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

# Discover direct child stacks; symlink location determines the stack root.
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

# Build <command> <user args> <injected flags>.
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
  # direnv loads the environment but does not change directory.
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
