#!/bin/bash
set -euo pipefail

# template_lib.sh — Common logic for k0s cluster management

# Prevent double sourcing.
if [[ "${TEMPLATE_LIB_LOADED:-0}" -eq 1 ]]; then
    return 0
fi
TEMPLATE_LIB_LOADED=1

export RED='\033[0;31m'
export GREEN='\033[0;32m'
export YELLOW='\033[1;33m'
export NC='\033[0m'

# Shared SSH identity for k0sctl and direct reboot calls; caller-overridable.
export K0S_SSH_KEY_PATH="${K0S_SSH_KEY_PATH:-$HOME/.ssh/id_ed25519}"

# ── logging ───────────────────────────────────────────────────────────────────

# Keep stdout machine-readable by logging to stderr.
log_error()   { echo -e "${RED}✗${NC} Error: $*" >&2; }
log_info()    { echo -e "${YELLOW}→${NC} $*" >&2; }
log_success() { echo -e "${GREEN}✓${NC} $*" >&2; }

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
    local script_name env_dir available_envs env_hint
    script_name=$(basename "$0")

    # Discover environments from env/*.sh.
    env_dir="${SCRIPT_DIR:-$(cd "$(dirname "$0")" && pwd)}/env"
    local available_envs="" f
    for f in "$env_dir"/*.sh; do
        [[ -f "$f" ]] || continue
        available_envs+="${available_envs:+|}$(basename "$f" .sh)"
    done
    env_hint="${available_envs:-<env>}"

    cat <<EOF
Usage: $script_name <${env_hint}> <command>

  To add a new environment, create env/<name>.sh (and optionally
  helmfile.<name>.yaml.gotmpl).

Commands:
  bootstrap   Create a new cluster: k0sctl apply (no wait) → kubeconfig → helmfile apply
  upgrade     Upgrade an existing cluster with readiness and storage health checks
  apply       Disabled legacy command; use bootstrap or upgrade explicitly
  reset       Reset cluster: k0sctl reset → reboot every node (K0S_SKIP_REBOOT=1 to skip)
  kubeconfig  Fetch kubeconfig to \$HOME/.kube/<env>.yaml
  helmfile    Apply helmfile only (requires kubeconfig to exist)
  smoke-test  Run smoke tests: L2LB reachability + PVC read/write (requires kubeconfig to exist)
  config      Print the bootstrap k0sctl config to stdout
  upgrade-config
              Print the upgrade k0sctl config to stdout
EOF
}

# ── preflight ─────────────────────────────────────────────────────────────────

preflight() {
    local cmd
    for cmd in k0sctl helmfile helm kubectl cilium ssh; do
        if ! command -v "$cmd" &>/dev/null; then
            log_error "required command '$cmd' not found in PATH"
            exit 1
        fi
    done

    validate_storage_config
}

validate_storage_config() {
    local providers="${K0S_STORAGE_PROVIDERS:-openebs}"
    local default_class="${K0S_DEFAULT_STORAGE_CLASS:-openebs-hostpath}"
    local provider expected_provider seen=","
    local -a provider_list

    if [[ "$providers" =~ [[:space:]] ]] || [[ "$providers" == ,* ]] ||
       [[ "$providers" == *, ]] || [[ "$providers" == *,,* ]]; then
        log_error "K0S_STORAGE_PROVIDERS must be a comma-separated list without spaces or empty entries: '$providers'"
        return 1
    fi

    IFS=',' read -ra provider_list <<< "$providers"
    for provider in "${provider_list[@]}"; do
        case "$provider" in
            openebs|nfs) ;;
            *)
                log_error "Unsupported storage provider in K0S_STORAGE_PROVIDERS: '$provider'"
                return 1
                ;;
        esac
        if [[ "$seen" == *",${provider},"* ]]; then
            log_error "Duplicate storage provider in K0S_STORAGE_PROVIDERS: '$provider'"
            return 1
        fi
        seen+="${provider},"
    done

    case "$default_class" in
        openebs-hostpath) expected_provider=openebs ;;
        nfs) expected_provider=nfs ;;
        *)
            log_error "Unsupported K0S_DEFAULT_STORAGE_CLASS: '$default_class'"
            return 1
            ;;
    esac
    if [[ ",$providers," != *",${expected_provider},"* ]]; then
        log_error "Default StorageClass '$default_class' requires provider '$expected_provider'"
        return 1
    fi

    if [[ ",$providers," == *",nfs,"* ]]; then
        validate_vars K0S_NFS_SERVER K0S_NFS_SHARE
        if [[ "${K0S_NFS_SHARE}" != /* ]]; then
            log_error "K0S_NFS_SHARE must be an absolute export path"
            return 1
        fi
    fi
}

# ── node addresses ────────────────────────────────────────────────────────────

# Print standard and optional GPU workers as a comma-separated list.
_worker_addresses() {
    validate_vars K0S_WORKER_ADDRESSES

    local addresses="${K0S_WORKER_ADDRESSES// /}"
    if [[ -n "${K0S_GPU_WORKER_ADDRESSES:-}" ]]; then
        addresses+=",${K0S_GPU_WORKER_ADDRESSES// /}"
    fi
    printf '%s' "$addresses"
}

# Print workers before controllers to preserve API access (ADR-0032).
_all_node_addresses() {
    validate_vars K0S_CONTROLLER_ADDRESSES

    printf '%s,%s' "$(_worker_addresses)" "${K0S_CONTROLLER_ADDRESSES// /}"
}

# ── k0sctl configuration ──────────────────────────────────────────────────────

_l2_segment_from_ip() {
    local addr="$1"
    local octet1 octet2 octet3 _octet4

    IFS='.' read -r octet1 octet2 octet3 _octet4 <<< "$addr"
    if [[ -z "$octet1" || -z "$octet2" || -z "$octet3" ]]; then
        log_error "Invalid worker IP address for L2 segment label: $addr"
        return 1
    fi

    printf '%s-%s-%s' "$octet1" "$octet2" "$octet3"
}

# Render a standard or GPU worker host.
# Usage: _render_worker_host <address> [gpu]
_render_worker_host() {
    local addr="$1"
    local kind="${2:-}"
    local labels l2_segment
    l2_segment="$(_l2_segment_from_ip "$addr")"
    labels="homelab/l2-segment=${l2_segment}"
    if [[ "$kind" == "gpu" ]]; then
        labels="${labels},gpu=amd"
    fi

    cat <<EOF
  - role: worker
    ssh:
      address: ${addr}
      user: ${K0S_SSH_USER}
      port: 22
      keyPath: ${K0S_SSH_KEY_PATH}
    installFlags:
      - --labels=${labels}
EOF

    if [[ "$kind" == "gpu" ]]; then
        cat <<EOF
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

# Add a CoreDNS-only GPU taint toleration so calculated replicas remain schedulable.
_render_coredns_config() {
    if [[ -z "${K0S_GPU_WORKER_ADDRESSES:-}" ]]; then
        return
    fi

    cat <<'EOF'
          coreDNS:
            patches:
              - target:
                  kind: Deployment
                  name: coredns
                  namespace: kube-system
                patch:
                  type: JSON
                  content: |
                    - op: add
                      path: /spec/template/spec/tolerations/-
                      value:
                        key: gpu
                        operator: Equal
                        value: amd
                        effect: NoSchedule
EOF
}

sync_worker_l2_labels() {
    local addresses node_table addr node_name l2_segment
    local -a addr_list
    addresses="$(_worker_addresses)"

    log_info "Syncing worker L2 segment labels..."
    node_table="$(kubectl get nodes -o wide --no-headers)"

    IFS=',' read -ra addr_list <<< "$addresses"
    for addr in "${addr_list[@]}"; do
        [[ -n "$addr" ]] || continue
        l2_segment="$(_l2_segment_from_ip "$addr")"
        node_name="$(awk -v ip="$addr" '$6 == ip { print $1; exit }' <<< "$node_table")"
        if [[ -z "$node_name" ]]; then
            log_info "Skipping L2 label for $addr; node is not registered yet"
            continue
        fi

        kubectl label node "$node_name" "homelab/l2-segment=${l2_segment}" --overwrite
    done
}

# Build k0sctl config from comma-separated environment topology.
#   K0S_CONTROLLER_ADDRESSES — required, comma-separated controller IPs
#   K0S_WORKER_ADDRESSES     — required, comma-separated worker IPs
#   K0S_GPU_WORKER_ADDRESSES — optional, comma-separated GPU worker IPs
#
# Datastore selection:
#   1 controller  → kine  (embedded SQLite, suitable for homelab single-node control plane)
#   2+ controllers → etcd (required for HA; controllers must be an odd number for quorum)
# Operation safety:
#   bootstrap → do not wait for Ready because the custom CNI is not installed yet
#   upgrade   → wait for each worker and allow storage recovery during drain
generate_k0sctl_config() {
    local k0sctl_file="$1"
    local operation_mode="${2:-bootstrap}"

    validate_vars K0S_SSH_USER K0S_CONTROLLER_ADDRESSES K0S_WORKER_ADDRESSES K0S_LB_POOL

    case "$operation_mode" in
        bootstrap|upgrade) ;;
        *)
            log_error "Unknown k0sctl operation mode: '$operation_mode'"
            return 1
            ;;
    esac

    log_info "Generating k0sctl configuration for ${operation_mode}..."

    # Split address lists and trim surrounding spaces.
    IFS=',' read -ra ctrl_list   <<< "${K0S_CONTROLLER_ADDRESSES// /}"
    IFS=',' read -ra worker_list <<< "${K0S_WORKER_ADDRESSES// /}"

    local ctrl_count="${#ctrl_list[@]}"
    local worker_count="${#worker_list[@]}"

    # Select datastore.
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
      keyPath: ${K0S_SSH_KEY_PATH}
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
${K0S_VERSION:+    version: ${K0S_VERSION}
}    config:
      spec:
        storage:
          type: ${storage_type} ${storage_comment}
        # Bind control-plane metrics off-loopback for Prometheus.
        controllerManager:
          extraArgs:
            bind-address: 0.0.0.0
        scheduler:
          extraArgs:
            bind-address: 0.0.0.0
        network:
          provider: custom # Set to custom to use Cilium
          kubeProxy: # Disable kube-proxy since Cilium provides kube-proxy replacement
            disabled: true
EOF

        # CoreDNS must explicitly tolerate the dedicated GPU worker taint.
        _render_coredns_config

        if [[ "$operation_mode" == "upgrade" ]]; then
            cat <<EOF
  options:
    wait:
      enabled: true
    drain:
      enabled: true
      gracePeriod: 2m
      timeout: 20m
      force: true
      ignoreDaemonSets: true
      deleteEmptyDirData: true
    concurrency:
      workerDisruptionPercent: 10
EOF
        else
            cat <<EOF
  options:
    wait:
      enabled: false
EOF
        fi
    } > "$k0sctl_file"

    local gpu_count=0
    if [[ -n "${K0S_GPU_WORKER_ADDRESSES:-}" ]]; then
        IFS=',' read -ra _gpu_list <<< "${K0S_GPU_WORKER_ADDRESSES// /}"
        gpu_count="${#_gpu_list[@]}"
    fi
    log_success "Configuration generated for ${operation_mode} — controllers: ${ctrl_count}, workers: ${worker_count}, gpu-workers: ${gpu_count}"
}

# ── kubeconfig ────────────────────────────────────────────────────────────────

generate_kubeconfig() {
    local k0sctl_file="$1"
    local kubeconfig_out="$2"

    # Reuse config generated earlier in this run.
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

# ── existing-cluster health checks ───────────────────────────────────────────

wait_for_nodes_ready() {
    local timeout="${1:-5m}"

    log_info "Waiting for all nodes to be Ready (timeout: ${timeout})..."
    kubectl wait --for=condition=Ready nodes --all --timeout="$timeout"
    log_success "All nodes are Ready"
}

wait_for_openebs_ready() {
    local timeout_seconds="${1:-300}"

    log_info "Waiting for OpenEBS pods to be Ready..."
    kubectl -n openebs wait \
        --for=condition=Ready pod \
        --all \
        --timeout="${timeout_seconds}s"
    log_success "OpenEBS pods are Ready"
}

wait_for_nfs_ready() {
    local timeout_seconds="${1:-300}"

    log_info "Waiting for NFS CSI controller and node plugin to be Ready..."
    kubectl -n kube-system rollout status deployment/csi-nfs-controller \
        --timeout="${timeout_seconds}s"
    kubectl -n kube-system rollout status daemonset/csi-nfs-node \
        --timeout="${timeout_seconds}s"
    log_success "NFS CSI pods are Ready"
}

wait_for_storage_ready() {
    local timeout_seconds="${1:-300}"
    local provider
    local -a provider_list

    IFS=',' read -ra provider_list <<< "${K0S_STORAGE_PROVIDERS:-openebs}"
    for provider in "${provider_list[@]}"; do
        case "$provider" in
            openebs) wait_for_openebs_ready "$timeout_seconds" ;;
            nfs) wait_for_nfs_ready "$timeout_seconds" ;;
        esac
    done
}

check_default_storage_classes() {
    local expected="${1:-}"
    local defaults

    defaults="$(
        kubectl get storageclass --request-timeout=10s \
            -o 'custom-columns=NAME:.metadata.name,DEFAULT:.metadata.annotations.storageclass\.kubernetes\.io/is-default-class' \
            --no-headers | awk '$2 == "true" { print $1 }'
    )"
    if [[ "$(wc -l <<< "$defaults" | tr -d ' ')" -gt 1 ]]; then
        log_error "Multiple default StorageClasses are configured:"
        printf '%s\n' "$defaults" >&2
        return 1
    fi
    if [[ -n "$expected" && "$defaults" != "$expected" ]]; then
        log_error "Expected default StorageClass '$expected', found '${defaults:-none}'"
        return 1
    fi
}

check_existing_cluster_health() {
    local timeout="${1:-5m}"
    local storage_timeout_seconds="${2:-300}"

    log_info "Checking existing cluster health..."
    kubectl get nodes >/dev/null
    wait_for_nodes_ready "$timeout"
    cilium status --wait
    wait_for_storage_ready "$storage_timeout_seconds"
    log_success "Existing cluster health checks passed"
}

# ── node reboot ───────────────────────────────────────────────────────────────

_ssh_node() {
    local addr="$1"
    shift

    # Accept new keys non-interactively, but reject changed keys from recreated VMs.
    ssh -o BatchMode=yes \
        -o StrictHostKeyChecking=accept-new \
        -o ConnectTimeout=10 \
        -i "$K0S_SSH_KEY_PATH" \
        "${K0S_SSH_USER}@${addr}" "$@"
}

_node_boot_id() {
    _ssh_node "$1" cat /proc/sys/kernel/random/boot_id
}

# Wait for a changed boot ID; SSH alone can still reach the pre-reboot daemon.
wait_for_reboot() {
    local addr="$1"
    local previous_boot_id="$2"
    local timeout="${3:-600}"
    local interval=5
    local elapsed=0
    local boot_id

    log_info "Waiting for $addr to come back..."
    while true; do
        boot_id="$(_node_boot_id "$addr" 2>/dev/null || true)"
        if [[ -n "$boot_id" && "$boot_id" != "$previous_boot_id" ]]; then
            log_success "$addr is back up"
            return 0
        fi
        if [[ "$elapsed" -ge "$timeout" ]]; then
            log_error "Timeout waiting for $addr to come back after reboot"
            return 1
        fi
        sleep "$interval"
        elapsed=$((elapsed + interval))
    done
}

# Reboot all nodes after reset to clear Cilium, nftables and kubelet residue.
# Trigger all first for parallel boot; the cluster no longer exists (ADR-0032).
reboot_nodes() {
    local timeout="${1:-600}"
    local addr boot_id i failed=""
    local -a addr_list boot_ids

    IFS=',' read -ra addr_list <<< "$(_all_node_addresses)"

    # Abort before changes if any initial boot ID cannot be read.
    for i in "${!addr_list[@]}"; do
        addr="${addr_list[i]}"
        if ! boot_id="$(_node_boot_id "$addr")"; then
            log_error "Cannot read the boot id of $addr over SSH"
            return 1
        fi
        boot_ids[i]="$boot_id"
    done

    for i in "${!addr_list[@]}"; do
        addr="${addr_list[i]}"
        log_info "Triggering reboot on $addr"
        # Detach reboot so SSH closure is not reported as failure.
        if ! _ssh_node "$addr" \
            "sudo systemd-run --on-active=3 --timer-property=AccuracySec=100ms systemctl reboot"; then
            log_error "Failed to trigger a reboot on $addr"
            failed+="${failed:+ }$addr"
            # Skip wait when reboot dispatch failed and cleared the boot ID.
            boot_ids[i]=""
        fi
    done

    # Collect all wait failures instead of stopping at the first.
    for i in "${!addr_list[@]}"; do
        [[ -n "${boot_ids[i]}" ]] || continue
        addr="${addr_list[i]}"
        if ! wait_for_reboot "$addr" "${boot_ids[i]}" "$timeout"; then
            failed+="${failed:+ }$addr"
        fi
    done

    if [[ -n "$failed" ]]; then
        log_error "Nodes that did not reboot cleanly: $failed"
        return 1
    fi

    log_success "All nodes rebooted"
}

# ── helmfile ──────────────────────────────────────────────────────────────────

helmfile_apply() {
    local helmfile_file="$1"
    local base_dir
    base_dir="$(dirname "$helmfile_file")"
    log_info "Using KUBECONFIG: ${KUBECONFIG:-unknown}"

    # Reject multiple defaults; deliberate switches may begin with zero.
    check_default_storage_classes

    sync_worker_l2_labels

    # Phase 1: install Cilium before live validation of CRD-dependent releases.
    log_info "Running: helmfile apply (phase 1: cilium)"
    helmfile -f "$helmfile_file" -l name=cilium apply

    # Wait until Cilium CRDs are established for helm-diff.
    "$base_dir/scripts/wait-cilium-crds.sh"

    # Phase 2: apply all releases after CRD readiness.
    log_info "Running: helmfile apply (phase 2: all releases)"
    helmfile -f "$helmfile_file" apply
    check_default_storage_classes "$K0S_DEFAULT_STORAGE_CLASS"
}

# ── gateway API CRDs ──────────────────────────────────────────────────────────

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
        bootstrap)
            generate_k0sctl_config "$k0sctl_file" bootstrap
            log_info "Running: k0sctl apply --config $k0sctl_file"
            k0sctl apply --config "$k0sctl_file"
            generate_kubeconfig "$k0sctl_file" "$kubeconfig_out"
            export KUBECONFIG="$kubeconfig_out"
            wait_for_cluster
            helmfile_apply "$helmfile_file"
            cilium status --wait
            wait_for_nodes_ready 5m
            wait_for_storage_ready 1200
            log_success "Cluster bootstrap completed successfully!"
            ;;
        upgrade)
            validate_file_exists "$kubeconfig_out" "Kubeconfig"
            export KUBECONFIG="$kubeconfig_out"
            check_existing_cluster_health 5m 300

            generate_k0sctl_config "$k0sctl_file" upgrade
            log_info "Running: k0sctl apply --config $k0sctl_file"
            k0sctl apply --config "$k0sctl_file"
            generate_kubeconfig "$k0sctl_file" "$kubeconfig_out"

            wait_for_nodes_ready 20m
            wait_for_storage_ready 1200
            helmfile_apply "$helmfile_file"
            cilium status --wait
            wait_for_nodes_ready 5m
            wait_for_storage_ready 1200
            log_success "Cluster upgrade completed successfully!"
            ;;
        apply)
            log_error "The ambiguous 'apply' command is disabled."
            log_error "Use 'bootstrap' for a new cluster or 'upgrade' for an existing cluster."
            return 2
            ;;
        reset)
            generate_k0sctl_config "$k0sctl_file" bootstrap
            log_info "Running: k0sctl reset --config $k0sctl_file"
            k0sctl reset --config "$k0sctl_file"
            if [[ "${K0S_SKIP_REBOOT:-0}" == "1" ]]; then
                log_info "K0S_SKIP_REBOOT=1 — nodes left running; reboot them before the next bootstrap"
            else
                reboot_nodes "${K0S_REBOOT_TIMEOUT:-600}"
            fi
            log_success "Cluster reset completed successfully!"
            ;;
        kubeconfig)
            generate_kubeconfig "$k0sctl_file" "$kubeconfig_out"
            ;;
        helmfile)
            export KUBECONFIG="$kubeconfig_out"
            helmfile_apply "$helmfile_file"
            ;;
        smoke-test)
            export KUBECONFIG="$kubeconfig_out"
            "$base_dir/tests/smoke-test.sh"
            ;;
        config)
            generate_k0sctl_config "$k0sctl_file" bootstrap
            cat "$k0sctl_file"
            ;;
        upgrade-config)
            generate_k0sctl_config "$k0sctl_file" upgrade
            cat "$k0sctl_file"
            ;;
        *)
            log_error "Unknown command: $command"
            usage
            exit 2
            ;;
    esac
}
