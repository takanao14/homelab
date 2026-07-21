#!/usr/bin/env bash
set -euo pipefail

# Remove stale SSH host keys for every node in a k0s environment.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

usage() {
    local script_name available_envs="" env_file
    script_name="$(basename "$0")"

    for env_file in "$SCRIPT_DIR"/env/*.sh; do
        [[ -f "$env_file" ]] || continue
        available_envs+="${available_envs:+|}$(basename "$env_file" .sh)"
    done

    cat <<EOF
Usage: $script_name <${available_envs:-env}>

Remove SSH known_hosts entries for all controller, worker, and GPU worker
addresses configured in k0s/env/<env>.sh.
EOF
}

if [[ $# -ne 1 ]]; then
    usage >&2
    exit 2
fi

ENV_TARGET="$1"
ENV_FILE="$SCRIPT_DIR/env/$ENV_TARGET.sh"

if [[ ! -f "$ENV_FILE" ]]; then
    echo "Error: unknown environment '$ENV_TARGET' (file not found: $ENV_FILE)" >&2
    usage >&2
    exit 2
fi

if ! command -v ssh-keygen &>/dev/null; then
    echo "Error: required command 'ssh-keygen' not found in PATH" >&2
    exit 1
fi

KNOWN_HOSTS_FILE="${KNOWN_HOSTS_FILE:-$HOME/.ssh/known_hosts}"
if [[ ! -f "$KNOWN_HOSTS_FILE" ]]; then
    echo "No known_hosts file found at $KNOWN_HOSTS_FILE; nothing to remove."
    exit 0
fi

# shellcheck disable=SC1090
source "$ENV_FILE"

if [[ -z "${K0S_CONTROLLER_ADDRESSES:-}" || -z "${K0S_WORKER_ADDRESSES:-}" ]]; then
    echo "Error: K0S_CONTROLLER_ADDRESSES and K0S_WORKER_ADDRESSES must be set in $ENV_FILE" >&2
    exit 1
fi

addresses="${K0S_CONTROLLER_ADDRESSES},${K0S_WORKER_ADDRESSES}"
if [[ -n "${K0S_GPU_WORKER_ADDRESSES:-}" ]]; then
    addresses+=",${K0S_GPU_WORKER_ADDRESSES}"
fi

declare -A seen=()
IFS=',' read -ra address_list <<< "$addresses"

for address in "${address_list[@]}"; do
    address="${address//[[:space:]]/}"
    [[ -n "$address" ]] || continue
    [[ -z "${seen[$address]:-}" ]] || continue
    seen["$address"]=1

    echo "Removing $address from known_hosts..."
    ssh-keygen -f "$KNOWN_HOSTS_FILE" -R "$address"
done
