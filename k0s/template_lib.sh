#!/bin/bash
set -euo pipefail

# makedev template library
# Expected to be `source`d by `makedev.sh` or `makeprd.sh`.
#
# Provided functions:
#  - usage, preflight, generate_config, run_apply, run_reset, run_kubeconfig, run_helmfile, run_main

# Prevent double-sourcing
if [ "${MAKEDEV_LIB_LOADED:-0}" -eq 1 ]; then
    return 0
fi
MAKEDEV_LIB_LOADED=1

usage() {
    local output_file="$1"
    local kubeconfig_out="$2"
    cat <<EOF
Usage: $0 <command>
Commands:
  apply       Generate config and run: k0sctl apply --config $output_file
              Then fetch kubeconfig and run helmfile apply.
  reset       Run: k0sctl reset --config $output_file
  kubeconfig  Generate config and output kubeconfig to $kubeconfig_out
  helmfile    Run: helmfile apply (uses env vars from script)
  gen         Only generate $output_file from template
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

generate_config() {
    local template_file="$1"
    local output_file="$2"

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

    if envsubst < "$template_file" > "$output_file"; then
        echo -e "${GREEN:-}✓${NC:-} Configuration generated: $output_file"
    else
        echo -e "${RED:-}✗${NC:-} Error: Failed to generate configuration"
        exit 1
    fi
}

run_apply() {
    local template_file="$1"
    local output_file="$2"

    generate_config "$template_file" "$output_file"
    echo -e "${YELLOW:-}→${NC:-} Running: k0sctl apply --config $output_file"
    k0sctl apply --config "$output_file"
}

run_reset() {
    local template_file="$1"
    local output_file="$2"

    # If config doesn't exist, generate it so k0sctl knows targets
    if [ ! -f "$output_file" ]; then
        generate_config "$template_file" "$output_file"
    fi
    echo -e "${YELLOW:-}→${NC:-} Running: k0sctl reset --config $output_file"
    k0sctl reset --config "$output_file"
}

run_kubeconfig() {
    local template_file="$1"
    local output_file="$2"
    local kubeconfig_out="$3"

    generate_config "$template_file" "$output_file"
    echo -e "${YELLOW:-}→${NC:-} Fetching kubeconfig via k0sctl"
    mkdir -p "$(dirname "$kubeconfig_out")"
    # k0sctl kubeconfig writes to stdout; capture to file
    k0sctl kubeconfig --config "$output_file" > "$kubeconfig_out"
    echo -e "${GREEN:-}✓${NC:-} kubeconfig written to: $kubeconfig_out"
}

run_helmfile() {
    echo -e "${YELLOW:-}→${NC:-} Running: helmfile apply"
    echo -e "${YELLOW:-}→${NC:-} Using KUBECONFIG: ${KUBECONFIG:-unknown}"

    if ! command -v helmfile &>/dev/null; then
        echo -e "${RED:-}✗${NC:-} Error: helmfile not found in PATH"
        exit 1
    fi

    # helmfile will use the environment variables exported by makedev.sh/makeprd.sh
    helmfile apply
}

run_main() {
    local template_file="$1"
    local output_file="$2"
    local kubeconfig_out="$3"
    shift 3
    local command="${1:-}"

    if [ -z "$command" ]; then
        usage "$output_file" "$kubeconfig_out"
        exit 1
    fi

    preflight

    case "$command" in
        apply)
            run_apply "$template_file" "$output_file"
            run_kubeconfig "$template_file" "$output_file" "$kubeconfig_out"
            export KUBECONFIG="$kubeconfig_out"
            run_helmfile
            echo -e "${GREEN:-}✓${NC:-} Cluster setup completed successfully!"
            ;;
        reset)
            run_reset "$template_file" "$output_file"
            ;;
        kubeconfig)
            run_kubeconfig "$template_file" "$output_file" "$kubeconfig_out"
            ;;
        helmfile)
            export KUBECONFIG="$kubeconfig_out"
            run_helmfile
            ;;
        gen)
            generate_config "$template_file" "$output_file"
            ;;
        help|-h|--help)
            usage "$output_file" "$kubeconfig_out"
            ;;
        *)
            echo -e "${RED:-}✗${NC:-} Unknown command: $command"
            usage "$output_file" "$kubeconfig_out"
            exit 2
            ;;
    esac
}
