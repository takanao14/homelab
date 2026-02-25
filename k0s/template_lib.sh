#!/bin/bash
set -euo pipefail

# template_lib.sh
# Common logic for k0s cluster management

# Prevent double-sourcing
if [ "${TEMPLATE_LIB_LOADED:-0}" -eq 1 ]; then
    return 0
fi
TEMPLATE_LIB_LOADED=1

# Color output (ensure these are defined if this script is sourced)
export RED='\033[0;31m'
export GREEN='\033[0;32m'
export YELLOW='\033[1;33m'
export NC='\033[0m'

usage() {
    local script_name
    script_name=$(basename "$0")

    cat <<EOF
Usage: $script_name <dev|prd> <command>

Commands:
  apply       Generate config and run: k0sctl apply
              Then fetch kubeconfig and run helmfile apply.
  reset       Run: k0sctl reset
  kubeconfig  Generate config and output kubeconfig
  helmfile    Run: helmfile apply (uses env vars from script)
  config      Only generate k0sctl config from template
  help        Show this message
EOF
}

preflight() {
    # Check required commands
    for cmd in envsubst k0sctl; do
        if ! command -v "$cmd" &>/dev/null; then
            echo -e "${RED:-}✗${NC:-} Error: required command '$cmd' not found in PATH"
            exit 1
        fi
    done
}

generate_k0sctl_config() {
    local template_file="$1"
    local k0sctl_file="$2"

    echo -e "${YELLOW:-}→${NC:-} Generating k0sctl configuration..."

    # Verify template file exists
    if [ ! -f "$template_file" ]; then
        echo -e "${RED:-}✗${NC:-} Error: Template file '$template_file' not found"
        exit 1
    fi

    # Validate required variables (checked via env)
    for var in K0S_SSH_USER K0S_CONTROLLER_ADDRESS K0S_WORKER_ADDRESS K0S_CLUSTER_NAME; do
        if [ -z "${!var:-}" ]; then
            echo -e "${RED:-}✗${NC:-} Error: Required variable $var is empty"
            exit 1
        fi
    done

    if envsubst < "$template_file" > "$k0sctl_file"; then
        echo -e "${GREEN:-}✓${NC:-} Configuration generated: $k0sctl_file"
    else
        echo -e "${RED:-}✗${NC:-} Error: Failed to generate configuration"
        exit 1
    fi
}

k0sctl_apply() {
    local template_file="$1"
    local k0sctl_file="$2"

    generate_k0sctl_config "$template_file" "$k0sctl_file"
    echo -e "${YELLOW:-}→${NC:-} Running: k0sctl apply --config $k0sctl_file"
    k0sctl apply --config "$k0sctl_file"
}

k0sctl_reset() {
    local template_file="$1"
    local k0sctl_file="$2"

    # If config doesn't exist, generate it so k0sctl knows targets
    if [ ! -f "$k0sctl_file" ]; then
        generate_k0sctl_config "$template_file" "$k0sctl_file"
    fi
    echo -e "${YELLOW:-}→${NC:-} Running: k0sctl reset --config $k0sctl_file"
    k0sctl reset --config "$k0sctl_file"
}

generate_kubeconfig() {
    local template_file="$1"
    local k0sctl_file="$2"
    local kubeconfig_out="$3"

    generate_k0sctl_config "$template_file" "$k0sctl_file"
    echo -e "${YELLOW:-}→${NC:-} Fetching kubeconfig via k0sctl"
    mkdir -p "$(dirname "$kubeconfig_out")"
    # k0sctl kubeconfig writes to stdout; capture to file
    k0sctl kubeconfig --config "$k0sctl_file" > "$kubeconfig_out"
    echo -e "${GREEN:-}✓${NC:-} kubeconfig written to: $kubeconfig_out"
}

helmfile_apply() {
    echo -e "${YELLOW:-}→${NC:-} Running: helmfile apply"
    echo -e "${YELLOW:-}→${NC:-} Using KUBECONFIG: ${KUBECONFIG:-unknown}"

    if ! command -v helmfile &>/dev/null; then
        echo -e "${RED:-}✗${NC:-} Error: helmfile not found in PATH"
        exit 1
    fi

    # helmfile will use the environment variables exported by the caller script
    helmfile apply
}

run_main() {
    local template_file="$1"
    local k0sctl_file="$2"
    local kubeconfig_out="$3"
    local command="${4:-}"

    if [ -z "$command" ]; then
        usage
        exit 1
    fi

    case "$command" in
        help|-h|--help)
            usage
            exit 0
            ;;
    esac

    preflight

    case "$command" in
        apply)
            k0sctl_apply "$template_file" "$k0sctl_file"
            generate_kubeconfig "$template_file" "$k0sctl_file" "$kubeconfig_out"
            export KUBECONFIG="$kubeconfig_out"
            helmfile_apply
            echo -e "${GREEN:-}✓${NC:-} Cluster setup completed successfully!"
            ;;
        reset)
            k0sctl_reset "$template_file" "$k0sctl_file"
            ;;
        kubeconfig)
            generate_kubeconfig "$template_file" "$k0sctl_file" "$kubeconfig_out"
            ;;
        helmfile)
            export KUBECONFIG="$kubeconfig_out"
            helmfile_apply
            ;;
        config)
            generate_k0sctl_config "$template_file" "$k0sctl_file"
            ;;
        *)
            echo -e "${RED:-}✗${NC:-} Unknown command: $command"
            usage
            exit 2
            ;;
    esac
}
