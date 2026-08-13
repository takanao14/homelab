#!/usr/bin/env bash
#
# Refresh all provider locks using each stack's direnv credentials.
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

for cmd in direnv terragrunt; do
  if ! command -v "${cmd}" >/dev/null 2>&1; then
    echo "Error: required command not found: ${cmd}" >&2
    exit 1
  fi
done

STACKS=()
while IFS= read -r terragrunt_file; do
  STACKS+=("$(dirname "${terragrunt_file}")")
done < <(
  find "${SCRIPT_DIR}" \
    -path '*/.terragrunt-cache' -prune \
    -o -name terragrunt.hcl -type f -print \
    | sort
)

if [[ ${#STACKS[@]} -eq 0 ]]; then
  echo "Error: no terragrunt stacks found under ${SCRIPT_DIR}" >&2
  exit 1
fi

for stack in "${STACKS[@]}"; do
  rel_stack="${stack#"${SCRIPT_DIR}/"}"
  echo "=== ${rel_stack} ==="

  (
    cd "${stack}"
    direnv exec "${stack}" terragrunt run -- init -upgrade
    direnv exec "${stack}" terragrunt run -- providers lock \
      -platform=darwin_arm64 \
      -platform=linux_amd64
  )
done
