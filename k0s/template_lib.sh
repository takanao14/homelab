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

# ============================================================================
# Logging utilities
# ============================================================================

log_error() {
    echo -e "${RED:-}✗${NC:-} Error: $*" >&2
}

log_info() {
    echo -e "${YELLOW:-}→${NC:-} $*"
}

log_success() {
    echo -e "${GREEN:-}✓${NC:-} $*"
}

# ============================================================================
# Validation utilities
# ============================================================================

validate_file_exists() {
    local file="$1"
    local name="${2:-File}"

    if [ ! -f "$file" ]; then
        log_error "$name '$file' not found"
        return 1
    fi
}

validate_vars() {
    local -a vars=("$@")

    for var in "${vars[@]}"; do
        if [ -z "${!var:-}" ]; then
            log_error "Required variable $var is empty"
            return 1
        fi
    done
}

# ============================================================================
# Usage
# ============================================================================

usage() {
    local script_name
    script_name=$(basename "$0")

    cat <<EOF
Usage: $script_name <dev|prd> <command>

Commands:
  apply       Full cluster setup: k0sctl apply → kubeconfig → helmfile apply → gateway-api CRDs
  reset       Reset cluster: k0sctl reset
  kubeconfig  Generate and output kubeconfig to \$HOME/.kube/<env>.yaml
  helmfile    Apply helmfile only (requires kubeconfig to exist)
  gateway-api Apply Gateway API CRDs only (requires kubeconfig to exist)
  config      Generate k0sctl config from template and print to stdout
  help        Show this message
EOF
}

# ============================================================================
# Preflight checks
# ============================================================================

preflight() {
    # Verify all required commands are available in PATH
    for cmd in envsubst k0sctl helmfile helm kubectl cilium; do
        if ! command -v "$cmd" &>/dev/null; then
            log_error "required command '$cmd' not found in PATH"
            exit 1
        fi
    done
}

# ============================================================================
# k0sctl configuration
# ============================================================================

generate_k0sctl_config() {
    local template_file="$1"
    local k0sctl_file="$2"

    log_info "Generating k0sctl configuration..."

    if ! validate_file_exists "$template_file" "Template file"; then
        exit 1
    fi

    # Validate all required environment variables are set
    # These variables are expected to be exported by create_cluster.sh
    if ! validate_vars K0S_SSH_USER K0S_CONTROLLER_ADDRESS K0S_WORKER_ADDRESS K0S_CLUSTER_NAME; then
        exit 1
    fi

    # Substitute environment variables in template and write to k0sctl_file
    if envsubst < "$template_file" > "$k0sctl_file"; then
        log_success "Configuration generated: $k0sctl_file"
    else
        log_error "Failed to generate configuration"
        exit 1
    fi
}

k0sctl_apply() {
    local template_file="$1"
    local k0sctl_file="$2"

    # Generate k0sctl config from template
    generate_k0sctl_config "$template_file" "$k0sctl_file"

    # Execute k0sctl apply to set up cluster
    log_info "Running: k0sctl apply --config $k0sctl_file"
    k0sctl apply --config "$k0sctl_file"
}

k0sctl_reset() {
    local template_file="$1"
    local k0sctl_file="$2"

    generate_k0sctl_config "$template_file" "$k0sctl_file"

    # Reset cluster and remove all nodes
    log_info "Running: k0sctl reset --config $k0sctl_file"
    k0sctl reset --config "$k0sctl_file"
}

# ============================================================================
# kubeconfig generation
# ============================================================================

generate_kubeconfig() {
    local template_file="$1"
    local k0sctl_file="$2"
    local kubeconfig_out="$3"

    # Only generate k0sctl config if the file is missing or empty
    if [ ! -s "$k0sctl_file" ]; then
        generate_k0sctl_config "$template_file" "$k0sctl_file"
    fi

    # Fetch kubeconfig from the cluster
    log_info "Fetching kubeconfig via k0sctl"
    mkdir -p "$(dirname "$kubeconfig_out")"
    # k0sctl kubeconfig writes to stdout; capture to file
    k0sctl kubeconfig --config "$k0sctl_file" > "$kubeconfig_out"
    log_success "kubeconfig written to: $kubeconfig_out"
}

# ============================================================================
# Helm/Helmfile
# ============================================================================

helmfile_apply() {
    local base_dir="$1"

    log_info "Running: helmfile apply"
    log_info "Using KUBECONFIG: ${KUBECONFIG:-unknown}"

    helmfile -f "$base_dir/helmfile.yaml" apply
}

# ============================================================================
# Gateway API CRDs
# ============================================================================

gateway_api_apply() {
    log_info "Applying Gateway API CRDs..."
    kubectl apply --server-side -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.5.0/standard-install.yaml
    log_success "Gateway API CRDs applied successfully!"
}

# ============================================================================
# Main command dispatcher
# ============================================================================

run_main() {
    # Arguments: command, base_dir, template_file, kubeconfig_out
    local command="${1:-}"
    local base_dir="${2:-}"
    local template_file="${3:-}"
    local kubeconfig_out="${4:-}"

    # Validate all arguments are provided
    if ! validate_vars command base_dir template_file kubeconfig_out; then
        usage
        exit 1
    fi

    # Handle help command
    case "$command" in
        help|-h|--help)
            usage
            exit 0
            ;;
    esac

    # Check that all required commands are available
    preflight

    # Allocate a temp file for the k0sctl config; clean up on any exit
    local k0sctl_file
    k0sctl_file=$(mktemp "$base_dir/k0sctl-XXXXXX")
    # shellcheck disable=SC2064
    trap "rm -f '$k0sctl_file'" EXIT

    # Dispatch to the appropriate command handler
    case "$command" in
        apply)
            # Full cluster setup: k0sctl → kubeconfig → helmfile → gateway-api → cilium
            k0sctl_apply "$template_file" "$k0sctl_file"
            generate_kubeconfig "$template_file" "$k0sctl_file" "$kubeconfig_out"
            export KUBECONFIG="$kubeconfig_out"
            helmfile_apply "$base_dir"
            gateway_api_apply
            cilium status --wait
            log_success "Cluster setup completed successfully!"
            ;;
        reset)
            # Tear down cluster
            k0sctl_reset "$template_file" "$k0sctl_file"
            ;;
        kubeconfig)
            # Fetch kubeconfig only
            generate_kubeconfig "$template_file" "$k0sctl_file" "$kubeconfig_out"
            ;;
        helmfile)
            # Apply helmfile only (requires existing kubeconfig)
            export KUBECONFIG="$kubeconfig_out"
            helmfile_apply "$base_dir"
            ;;
        gateway-api)
            # Apply Gateway API CRDs only (requires existing kubeconfig)
            export KUBECONFIG="$kubeconfig_out"
            gateway_api_apply
            ;;
        config)
            # Generate k0sctl config from template and print to stdout
            generate_k0sctl_config "$template_file" "$k0sctl_file"
            cat "$k0sctl_file"
            ;;
        *)
            log_error "Unknown command: $command"
            usage
            exit 2
            ;;
    esac
}
