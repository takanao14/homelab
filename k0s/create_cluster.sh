#!/bin/bash
set -euo pipefail

# create_cluster.sh — Entry point for k0s cluster management
# Usage: ./create_cluster.sh <dev|prd> <command>

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
SECRETS_FILE="$SCRIPT_DIR/secrets.$ENV_TARGET.enc.env"

_ENV_HELMFILE="$SCRIPT_DIR/helmfile.$ENV_TARGET.yaml.gotmpl"
HELMFILE_FILE="$( [[ -f "$_ENV_HELMFILE" ]] && echo "$_ENV_HELMFILE" || echo "$SCRIPT_DIR/helmfile.yaml.gotmpl" )"
KUBECONFIG_OUT="$HOME/.kube/$ENV_TARGET.yaml"

# Validate environment by checking whether the env file exists.
# To add a new environment, simply create env/<name>.sh — no code changes needed.
if [[ ! -f "$ENV_FILE" ]]; then
    log_error "Unknown environment: '$ENV_TARGET' (env file not found: $ENV_FILE)"
    usage
    exit 2
fi

# ── derive cluster name ───────────────────────────────────────────────────────

export K0S_CLUSTER_NAME="${ENV_TARGET}-homelab"

# ── load environment ──────────────────────────────────────────────────────────

set -a
# shellcheck disable=SC1090
source "$ENV_FILE"
if [[ -f "$SECRETS_FILE" ]]; then
    _tmp=$(mktemp)
    trap 'rm -f "$_tmp"' EXIT
    sops --decrypt "$SECRETS_FILE" > "$_tmp"
    # shellcheck disable=SC1090
    source "$_tmp"
fi
set +a

# ── dispatch ──────────────────────────────────────────────────────────────────

run_main "$COMMAND" "$SCRIPT_DIR" "$KUBECONFIG_OUT" "$HELMFILE_FILE"
