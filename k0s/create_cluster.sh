#!/bin/bash
set -euo pipefail

# Color output
export RED='\033[0;31m'
export GREEN='\033[0;32m'
export YELLOW='\033[1;33m'
export NC='\033[0m'

# Helper to show usage before sourcing lib if possible, or just basic usage
usage_stub() {
    echo "Usage: $(basename "$0") <dev|prd> <command> [args...]" >&2
}

if [ "${1:-}" = "" ]; then
    usage_stub
    exit 2
fi

ENV_TARGET="$1"
shift

case "$ENV_TARGET" in
    dev)
        ENV_FILE=".env.dev"
        K0SCTRL_FILE="dev-homelab-k0sctl.yaml"
        KUBECONFIG_OUT="$HOME/.kube/dev-homelab.yaml"
        ;;
    prd)
        ENV_FILE=".env.homelab"
        K0SCTRL_FILE="homelab-k0sctl.yaml"
        KUBECONFIG_OUT="$HOME/.kube/homelab.yaml"
        ;;
    *)
        usage_stub
        exit 2
        ;;
esac

# Load env file if it exists
if [ -f "$ENV_FILE" ]; then
    # shellcheck source=/dev/null
    set -a
    . "$ENV_FILE"
    set +a
else
    echo -e "${RED}✗${NC} Error: environment file not found: $ENV_FILE"
    exit 1
fi

export K0S_SSH_USER="${K0S_SSH_USER}"
export K0S_CONTROLLER_ADDRESS="${K0S_CONTROLLER_ADDRESS}"
export K0S_WORKER_ADDRESS="${K0S_WORKER_ADDRESS}"
export K0S_CLUSTER_NAME="${K0S_CLUSTER_NAME}"

TEMPLATE_FILE="k0sctl.tmpl.yaml"
LIB_FILE="$(dirname "$0")/template_lib.sh"

if [ -f "$LIB_FILE" ]; then
    # shellcheck source=/dev/null
    . "$LIB_FILE"
else
    echo -e "${RED}✗${NC} Error: library file not found: $LIB_FILE"
    exit 1
fi

# Entrypoint
run_main "$TEMPLATE_FILE" "$K0SCTRL_FILE" "$KUBECONFIG_OUT" "$@"
