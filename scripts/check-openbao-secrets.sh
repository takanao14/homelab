#!/usr/bin/env bash
set -euo pipefail

# Cross-check declared secret paths against what OpenBao actually stores.
#
# Reports, in both directions:
#   1. paths declared in SOPS but absent from the server, which the next
#      `ops-openbao_seed_secrets.yaml` run recreates
#   2. paths stored on the server that nothing declares
#
# Reports only. Destroying an orphan stays a deliberate `bao kv metadata delete`.
# Reads path names alone; secret values are never extracted.
# Needs the AGE key and network access to OpenBao, so it does not run in CI.
# Compatible with macOS' Bash 3.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/lib/openbao-auth.sh"

REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
INVENTORY="${REPO_ROOT}/ansible/inventories/homelab/group_vars/openbao.sops.yaml"

# Paths written by scripts/secrets/admin/, not by any playbook.
# Add an entry here whenever a new set-*.sh writes to a new path.
UNMANAGED_PATHS="
secret/sops/age
secret/provision/env
secret/kubeconfig/prd
secret/kubeconfig/sandbox
"

BAO_ADDR="${OPENBAO_ADDR:-https://openbao.home.butaco.net}"
export BAO_ADDR

for cmd in bao jq sops yq; do
    if ! command -v "$cmd" &>/dev/null; then
        echo "Error: $cmd not found" >&2
        exit 2
    fi
done

if [[ ! -f "$INVENTORY" ]]; then
    echo "Error: ${INVENTORY} not found" >&2
    exit 2
fi

# Argo CD entries come from openbao_argocd_admin, which stores an env, not a path.
collect_declared() {
    echo "$UNMANAGED_PATHS"
    sops -d "$INVENTORY" | yq '.openbao_secrets[].path'
    sops -d "$INVENTORY" | yq '.openbao_argocd_admin[].env' |
        sed 's|^|secret/k8s/argocd/|; s|$|/admin|'
}

# KV v2 lists metadata, so a soft-deleted secret still counts as stored.
walk() {
    local prefix="$1" key
    while IFS= read -r key; do
        [[ -n "$key" ]] || continue
        case "$key" in
        */) walk "${prefix}${key}" ;;
        *) echo "secret/${prefix}${key}" ;;
        esac
    done < <(bao kv list -format=json "secret/${prefix}" 2>/dev/null | jq -r '.[]?')
}

BAO_USERNAME="${BAO_USERNAME:-homelab}"
openbao_authenticate

indent() {
    while IFS= read -r line; do
        echo "  $line"
    done
}

declared="$(mktemp)"
stored="$(mktemp)"
trap 'rm -f "$declared" "$stored"' EXIT

collect_declared | grep -v '^$' | sort -u >"$declared"
walk "" | sort -u >"$stored"

if [[ ! -s "$stored" ]]; then
    echo "Error: OpenBao returned no paths; check that the account can list secret/." >&2
    exit 2
fi

ghosts="$(comm -23 "$declared" "$stored")"
orphans="$(comm -13 "$declared" "$stored")"

status=0

if [[ -n "$ghosts" ]]; then
    echo "Declared but absent from OpenBao (the next seed run recreates these):"
    echo "$ghosts" | indent
    echo
    status=1
fi

if [[ -n "$orphans" ]]; then
    echo "Stored in OpenBao but declared nowhere:"
    echo "$orphans" | indent
    echo
    status=1
fi

echo "$(comm -12 "$declared" "$stored" | grep -c '' | tr -d ' ') paths matched."

exit "$status"
