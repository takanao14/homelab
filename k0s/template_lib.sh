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
    for cmd in k0sctl helmfile helm kubectl cilium; do
        if ! command -v "$cmd" &>/dev/null; then
            log_error "required command '$cmd' not found in PATH"
            exit 1
        fi
    done
}

# ── k0sctl configuration ──────────────────────────────────────────────────────

# Generates a worker host entry (standard or GPU).
# Usage: _render_worker_host <address> [gpu]
_render_worker_host() {
    local addr="$1"
    local kind="${2:-}"

    cat <<EOF
  - role: worker
    ssh:
      address: ${addr}
      user: ${K0S_SSH_USER}
      port: 22
      keyPath: ~/.ssh/id_ed25519
EOF

    if [[ "$kind" == "gpu" ]]; then
        cat <<EOF
    installFlags:
      - --labels=gpu=amd
      - --taints=gpu=amd:NoSchedule
EOF
    fi

    cat <<EOF
    files:
      - name: setup-ssd
        src: ./hook/ssdsetup.sh
        dstDir: /home/${K0S_SSH_USER}/k0sctl-hooks/
        perm: 0755
      - name: mirror-config
        src: ./hook/mirror.sh
        dstDir: /home/${K0S_SSH_USER}/k0sctl-hooks/
        perm: 0755
    hooks:
      apply:
        before:
          - /home/${K0S_SSH_USER}/k0sctl-hooks/ssdsetup.sh
          - /home/${K0S_SSH_USER}/k0sctl-hooks/mirror.sh
EOF
}

# Builds the full k0sctl config from environment variables.
# Supports multiple controllers and workers via comma-separated address lists.
#   K0S_CONTROLLER_ADDRESSES — required, comma-separated controller IPs
#   K0S_WORKER_ADDRESSES     — required, comma-separated worker IPs
#   K0S_GPU_WORKER_ADDRESSES — optional, comma-separated GPU worker IPs
#
# Storage backend is selected automatically:
#   1 controller  → kine  (embedded SQLite, suitable for homelab single-node control plane)
#   2+ controllers → etcd (required for HA; controllers must be an odd number for quorum)
generate_k0sctl_config() {
    local k0sctl_file="$1"

    validate_vars K0S_SSH_USER K0S_CONTROLLER_ADDRESSES K0S_WORKER_ADDRESSES K0S_LB_POOL

    log_info "Generating k0sctl configuration..."

    # Split address lists (trim spaces around commas)
    IFS=',' read -ra ctrl_list   <<< "${K0S_CONTROLLER_ADDRESSES// /}"
    IFS=',' read -ra worker_list <<< "${K0S_WORKER_ADDRESSES// /}"

    local ctrl_count="${#ctrl_list[@]}"
    local worker_count="${#worker_list[@]}"

    # Determine storage backend
    local storage_type storage_comment
    if [[ "$ctrl_count" -gt 1 ]]; then
        storage_type="etcd"
        storage_comment="# HA setup: etcd required for multiple controllers"
        log_info "Multiple controllers (${ctrl_count}) detected — using etcd storage backend"
        if (( ctrl_count % 2 == 0 )); then
            log_error "Controller count must be odd for etcd quorum (got ${ctrl_count})"
            return 1
        fi
    else
        storage_type="kine"
        storage_comment="# Use kine instead of etcd for homelab single-controller setup"
    fi

    {
        # ── header ──
        cat <<EOF
apiVersion: k0sctl.k0sproject.io/v1beta1
kind: Cluster
metadata:
  name: ${K0S_CLUSTER_NAME}
spec:
  hosts:
EOF

        # ── controller hosts ──
        for addr in "${ctrl_list[@]}"; do
            cat <<EOF
  - role: controller
    ssh:
      address: ${addr}
      user: ${K0S_SSH_USER}
      port: 22
      keyPath: ~/.ssh/id_ed25519
EOF
        done

        # ── standard worker hosts ──
        for addr in "${worker_list[@]}"; do
            _render_worker_host "$addr"
        done

        # ── GPU worker hosts (optional) ──
        if [[ -n "${K0S_GPU_WORKER_ADDRESSES:-}" ]]; then
            IFS=',' read -ra gpu_list <<< "${K0S_GPU_WORKER_ADDRESSES// /}"
            for addr in "${gpu_list[@]}"; do
                _render_worker_host "$addr" gpu
            done
        fi

        # ── k0s config ──
        cat <<EOF
  k0s:
    config:
      spec:
        storage:
          type: ${storage_type} ${storage_comment}
        network:
          provider: custom # Set to custom to use Cilium
          kubeProxy: # Disable kube-proxy since Cilium provides kube-proxy replacement
            disabled: true
          coreDNS:
            replicaCount: 1
  options:
    wait:
      enabled: false
EOF
    } > "$k0sctl_file"

    local gpu_count=0
    if [[ -n "${K0S_GPU_WORKER_ADDRESSES:-}" ]]; then
        IFS=',' read -ra _gpu_list <<< "${K0S_GPU_WORKER_ADDRESSES// /}"
        gpu_count="${#_gpu_list[@]}"
    fi
    log_success "Configuration generated — controllers: ${ctrl_count}, workers: ${worker_count}, gpu-workers: ${gpu_count}"
}

# ── kubeconfig ────────────────────────────────────────────────────────────────

generate_kubeconfig() {
    local k0sctl_file="$1"
    local kubeconfig_out="$2"

    # k0sctl_file is populated by generate_k0sctl_config; skip if already done (e.g. via apply)
    if [[ ! -s "$k0sctl_file" ]]; then
        generate_k0sctl_config "$k0sctl_file"
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

    log_info "Waiting for at least one worker node to register..."
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
    local helmfile_file="$1"
    log_info "Running: helmfile apply"
    log_info "Using KUBECONFIG: ${KUBECONFIG:-unknown}"
    helmfile -f "$helmfile_file" apply
}

# ── gateway API CRDs ──────────────────────────────────────────────────────────

gateway_api_apply() {
    # Gateway API CRD version must match what the installed Cilium version requires.
    # Check the supported version at: https://docs.cilium.io/en/stable/network/servicemesh/gateway-api/gateway-api/
    # Current: v1.4.1 experimental for Cilium 1.19.2
    log_info "Applying Gateway API CRDs..."
    kubectl apply --server-side -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.4.1/experimental-install.yaml
    log_success "Gateway API CRDs applied"
}

# ── command dispatcher ────────────────────────────────────────────────────────

run_main() {
    local command="$1"
    local base_dir="$2"
    local kubeconfig_out="$3"
    local helmfile_file="$4"

    preflight

    local k0sctl_file
    k0sctl_file=$(mktemp "$base_dir/k0sctl-XXXXXX")
    # shellcheck disable=SC2064
    trap "rm -f '$k0sctl_file'" EXIT

    case "$command" in
        apply)
            generate_k0sctl_config "$k0sctl_file"
            log_info "Running: k0sctl apply --config $k0sctl_file"
            k0sctl apply --config "$k0sctl_file"
            generate_kubeconfig "$k0sctl_file" "$kubeconfig_out"
            export KUBECONFIG="$kubeconfig_out"
            wait_for_cluster
            helmfile_apply "$helmfile_file"
            gateway_api_apply
            cilium status --wait
            log_success "Cluster setup completed successfully!"
            ;;
        reset)
            generate_k0sctl_config "$k0sctl_file"
            log_info "Running: k0sctl reset --config $k0sctl_file"
            k0sctl reset --config "$k0sctl_file"
            ;;
        kubeconfig)
            generate_kubeconfig "$k0sctl_file" "$kubeconfig_out"
            ;;
        helmfile)
            export KUBECONFIG="$kubeconfig_out"
            helmfile_apply "$helmfile_file"
            ;;
        gateway-api)
            export KUBECONFIG="$kubeconfig_out"
            gateway_api_apply
            ;;
        config)
            generate_k0sctl_config "$k0sctl_file"
            cat "$k0sctl_file"
            ;;
        *)
            log_error "Unknown command: $command"
            usage
            exit 2
            ;;
    esac
}
