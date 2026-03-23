#!/bin/bash
set -euo pipefail

# create_cluster.sh
# Entry point for k0s cluster management
# Usage: ./create_cluster.sh <dev|prd> <command>

# ============================================================================
# Constants
# ============================================================================

# Resolve script directory for consistent path references
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
TEMPLATE_FILE="$SCRIPT_DIR/k0sctl.tmpl.yaml"
LIB_FILE="$SCRIPT_DIR/template_lib.sh"

# Source library early so color vars and log helpers are available everywhere
if [ ! -f "$LIB_FILE" ]; then
    echo "Error: Library file not found: $LIB_FILE" >&2
    exit 1
fi
# shellcheck source=/dev/null
. "$LIB_FILE"

# ============================================================================
# Error handling utilities
# ============================================================================

die() {
    echo -e "${RED}✗${NC} Error: $*" >&2
    exit 1
}

usage() {
    echo "Usage: $(basename "$0") <dev|prd> <command>" >&2
}

# ============================================================================
# Environment configuration
# ============================================================================

setup_env_config() {
    local target="$1"

    # Map environment target to configuration
    case "$target" in
        dev)
            ENV_FILE="$SCRIPT_DIR/.env.dev"
            K0SCTRL_FILE="$SCRIPT_DIR/dev-homelab-k0sctl.yaml"
            KUBECONFIG_OUT="$HOME/.kube/dev-homelab.yaml"
            ;;
        prd)
            ENV_FILE="$SCRIPT_DIR/.env.homelab"
            K0SCTRL_FILE="$SCRIPT_DIR/homelab-k0sctl.yaml"
            KUBECONFIG_OUT="$HOME/.kube/homelab.yaml"
            ;;
        *)
            return 1
            ;;
    esac
}

# ============================================================================
# Main entry point
# ============================================================================

# Validate environment target argument
if [ "${1:-}" = "" ]; then
    usage
    exit 2
fi

ENV_TARGET="$1"
shift

# Setup environment configuration
if ! setup_env_config "$ENV_TARGET"; then
    usage
    die "Invalid environment target: $ENV_TARGET (must be dev or prd)"
fi

# Validate environment file exists
if [ ! -f "$ENV_FILE" ]; then
    die "Environment file not found: $ENV_FILE"
fi

# Source environment file with set -a to export all variables
set -a
# shellcheck disable=SC1090
source "$ENV_FILE"
set +a

# Validate command argument
if [ "${#@}" -eq 0 ]; then
    usage
    exit 2
fi

COMMAND="$1"

# Validate library file exists
if [ ! -f "$LIB_FILE" ]; then
    die "Library file not found: $LIB_FILE"
fi

# Source library
# shellcheck source=/dev/null
. "$LIB_FILE"

# Entrypoint: pass arguments in order: command, environment, script_dir, template_file, k0sctl_file, kubeconfig_out
run_main "$COMMAND" "$ENV_TARGET" "$SCRIPT_DIR" "$TEMPLATE_FILE" "$K0SCTRL_FILE" "$KUBECONFIG_OUT"
