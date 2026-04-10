#!/bin/bash
set -euo pipefail

# template_lib.sh — Common logic for k0s cluster management

# Prevent double-sourcing
if [[ "${TEMPLATE_LIB_LOADED:-0}" -eq 1 ]]; then
    return 0
fi
TEMPLATE_LIB_LOADED=1

export RED='\033[0;31m'
export GREEN='\033[0;32m'
export YELLOW='\033[1;33m'
export NC='\033[0m'

# ── logging ───────────────────────────────────────────────────────────────────

log_error()   { echo -e "${RED}✗${NC} Error: $*" >&2; }
log_info()    { echo -e "${YELLOW}→${NC} $*"; }
log_success() { echo -e "${GREEN}✓${NC} $*"; }

# ── validation ────────────────────────────────────────────────────────────────

validate_file_exists() {
    local file="$1" name="${2:-File}"
    if [[ ! -f "$file" ]]; then
        log_error "$name '$file' not found"
        return 1
    fi
}

validate_vars() {
    local var
    for var in "$@"; do
        if [[ -z "${!var:-}" ]]; then
            log_error "Required variable $var is empty"
            return 1
        fi
    done
}

# ── usage ─────────────────────────────────────────────────────────────────────

usage() {
    local script_name
    script_name=$(basename "$0")
    cat <<EOF
Usage: $script_name <dev|prd> <command>

Commands:
  apply       Full cluster setup: k0sctl apply → kubeconfig → helmfile apply → gateway-api CRDs
  reset       Reset cluster: k0sctl reset
  kubeconfig  Fetch kubeconfig to \$HOME/.kube/<env>.yaml
  helmfile    Apply helmfile only (requires kubeconfig to exist)
  gateway-api Apply Gateway API CRDs only (requires kubeconfig to exist)
  config      Print generated k0sctl config to stdout
EOF
}

# ── preflight ─────────────────────────────────────────────────────────────────

preflight() {
    local cmd
    for cmd in envsubst k0sctl helmfile helm kubectl cilium; do
        if ! command -v "$cmd" &>/dev/null; then
            log_error "required command '$cmd' not found in PATH"
            exit 1
        fi
    done
}

# ── k0sctl configuration ──────────────────────────────────────────────────────

generate_k0sctl_config() {
    local template_file="$1"
    local k0sctl_file="$2"

    log_info "Generating k0sctl configuration..."
    validate_file_exists "$template_file" "Template file"
    validate_vars K0S_SSH_USER K0S_CONTROLLER_ADDRESS K0S_WORKER_ADDRESS K0S_LB_POOL
    envsubst < "$template_file" > "$k0sctl_file"
    log_success "Configuration generated"
}

# ── kubeconfig ────────────────────────────────────────────────────────────────

generate_kubeconfig() {
    local template_file="$1"
    local k0sctl_file="$2"
    local kubeconfig_out="$3"

    # k0sctl_file is populated by generate_k0sctl_config; skip if already done (e.g. via apply)
    if [[ ! -s "$k0sctl_file" ]]; then
        generate_k0sctl_config "$template_file" "$k0sctl_file"
    fi

    log_info "Fetching kubeconfig via k0sctl"
    mkdir -p "$(dirname "$kubeconfig_out")"
    k0sctl kubeconfig --config "$k0sctl_file" > "$kubeconfig_out"
    log_success "kubeconfig written to: $kubeconfig_out"
}

# ── wait for cluster ──────────────────────────────────────────────────────────

wait_for_cluster() {
    local timeout=300
    local interval=5
    local elapsed=0

    log_info "Waiting for API server to be reachable..."
    until kubectl get nodes &>/dev/null; do
        if [[ "$elapsed" -ge "$timeout" ]]; then
            log_error "Timeout waiting for API server"
            return 1
        fi
        sleep "$interval"
        elapsed=$((elapsed + interval))
    done
    log_success "API server is reachable"

    log_info "Waiting for worker node to register..."
    until kubectl get nodes --no-headers 2>/dev/null | grep -qv "^$"; do
        if [[ "$elapsed" -ge "$timeout" ]]; then
            log_error "Timeout waiting for worker node"
            return 1
        fi
        sleep "$interval"
        elapsed=$((elapsed + interval))
    done
    log_success "Worker node registered (CNI not yet required)"
}

# ── helmfile ──────────────────────────────────────────────────────────────────

helmfile_apply() {
    local base_dir="$1"
    log_info "Running: helmfile apply"
    log_info "Using KUBECONFIG: ${KUBECONFIG:-unknown}"
    helmfile -f "$base_dir/helmfile.yaml" apply
}

# ── gateway API CRDs ──────────────────────────────────────────────────────────

gateway_api_apply() {
    log_info "Applying Gateway API CRDs..."
    kubectl apply --server-side -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.4.1/experimental-install.yaml
    log_success "Gateway API CRDs applied"
}

# ── command dispatcher ────────────────────────────────────────────────────────

run_main() {
    local command="$1"
    local base_dir="$2"
    local template_file="$3"
    local kubeconfig_out="$4"

    preflight

    local k0sctl_file
    k0sctl_file=$(mktemp "$base_dir/k0sctl-XXXXXX")
    # shellcheck disable=SC2064
    trap "rm -f '$k0sctl_file'" EXIT

    case "$command" in
        apply)
            generate_k0sctl_config "$template_file" "$k0sctl_file"
            log_info "Running: k0sctl apply --config $k0sctl_file"
            k0sctl apply --config "$k0sctl_file"
            generate_kubeconfig "$template_file" "$k0sctl_file" "$kubeconfig_out"
            export KUBECONFIG="$kubeconfig_out"
            wait_for_cluster
            helmfile_apply "$base_dir"
            gateway_api_apply
            cilium status --wait
            log_success "Cluster setup completed successfully!"
            ;;
        reset)
            generate_k0sctl_config "$template_file" "$k0sctl_file"
            log_info "Running: k0sctl reset --config $k0sctl_file"
            k0sctl reset --config "$k0sctl_file"
            ;;
        kubeconfig)
            generate_kubeconfig "$template_file" "$k0sctl_file" "$kubeconfig_out"
            ;;
        helmfile)
            export KUBECONFIG="$kubeconfig_out"
            helmfile_apply "$base_dir"
            ;;
        gateway-api)
            export KUBECONFIG="$kubeconfig_out"
            gateway_api_apply
            ;;
        config)
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
