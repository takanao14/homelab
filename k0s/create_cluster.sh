#!/bin/bash
set -euo pipefail

# k0s cluster management entry point.
# Usage: ./create_cluster.sh <prd|sandbox> <command>

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# shellcheck source=/dev/null
source "$SCRIPT_DIR/template_lib.sh"

# ── argument parsing ──────────────────────────────────────────────────────────

if [[ $# -lt 2 ]]; then
    usage
    exit 2
fi

ENV_TARGET="$1"
COMMAND="$2"

# ── paths ─────────────────────────────────────────────────────────────────────

ENV_FILE="$SCRIPT_DIR/env/$ENV_TARGET.sh"

_ENV_HELMFILE="$SCRIPT_DIR/helmfile.$ENV_TARGET.yaml.gotmpl"
HELMFILE_FILE="$( [[ -f "$_ENV_HELMFILE" ]] && echo "$_ENV_HELMFILE" || echo "$SCRIPT_DIR/helmfile.yaml.gotmpl" )"
KUBECONFIG_OUT="$HOME/.kube/$ENV_TARGET.yaml"

# Environments are discovered from env/<name>.sh.
if [[ ! -f "$ENV_FILE" ]]; then
    log_error "Unknown environment: '$ENV_TARGET' (env file not found: $ENV_FILE)"
    usage
    exit 2
fi

# ── derive cluster name ───────────────────────────────────────────────────────

export K0S_CLUSTER_NAME="${ENV_TARGET}-homelab"

# ── load environment ──────────────────────────────────────────────────────────

# Clear inherited topology before loading env/<target>.sh; preserve SSH user override.
unset K0S_VERSION \
      K0S_CONTROLLER_ADDRESSES \
      K0S_WORKER_ADDRESSES \
      K0S_GPU_WORKER_ADDRESSES \
      K0S_LB_POOL \
      K0S_STORAGE_PROVIDERS \
      K0S_DEFAULT_STORAGE_CLASS \
      K0S_NFS_SERVER \
      K0S_NFS_SHARE

set -a
# shellcheck disable=SC1090
source "$ENV_FILE"
K0S_SSH_USER="${K0S_SSH_USER:-$(id -un)}"
set +a

# ── dispatch ──────────────────────────────────────────────────────────────────

run_main "$COMMAND" "$SCRIPT_DIR" "$KUBECONFIG_OUT" "$HELMFILE_FILE"
