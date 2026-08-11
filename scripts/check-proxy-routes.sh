#!/usr/bin/env bash
set -euo pipefail

# Cross-check the Caddy reverse-proxy sites against the Homepage dashboard.
#
# Caddy publishes every *.home.butaco.net service, but nothing links the two
# sides: a site added to caddy_upstreams stays invisible on the portal until
# someone remembers to add it to services.yaml (this happened with code-server
# / vscode.home.butaco.net), and a dashboard tile can outlive the Caddy site it
# points at, leaving a dead link. This script fails fast in CI instead
# (.github/workflows/proxy-routes.yaml).
#
# Checks, in both directions:
#   1. every caddy_upstreams / caddy_redirects hostname has a Homepage entry,
#      except the machine-facing endpoints listed in NO_UI_HOSTS
#   2. every Homepage `https://<host>.home.butaco.net` link (no port, no plain
#      http) is a Caddy site
#
# Direction 2 keys on the URL shape rather than an exclusion list: an explicit
# port (Proxmox at :8006) or plain http (the network appliances) means the
# entry is reached directly and never traverses Caddy.
#
# Avoids bash-4-only features (mapfile, associative arrays) so it also runs
# on macOS' stock /bin/bash.

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

CADDY_VARS="${REPO_ROOT}/ansible/inventories/homelab/group_vars/caddy.yaml"
SERVICES="${REPO_ROOT}/k8s/homepage/chart/config/services.yaml"

# Only this zone is served by Caddy. *.prd / *.sandbox go through the
# in-cluster Gateway and are out of scope here.
ZONE="home.butaco.net"

# Caddy sites that intentionally have no dashboard tile: API endpoints with no
# UI to link to. The UI that fronts them is listed separately (SeaweedFS' own
# console is seaweedfs-ui). One hostname per line.
NO_UI_HOSTS="s3.${ZONE}"

# Hostnames published by Caddy (caddy_upstreams and caddy_redirects both key
# the site on `hostname`).
caddy_hosts="$(
    sed -n 's/^[[:space:]]*-\{0,1\}[[:space:]]*hostname:[[:space:]]*\([^[:space:]#]*\).*/\1/p' "$CADDY_VARS" |
        grep -F ".${ZONE}" | sort -u
)"

# Every host Homepage links to, port stripped.
homepage_hosts="$(
    sed -n 's|^[[:space:]]*href:[[:space:]]*https\{0,1\}://\([^/[:space:]]*\).*|\1|p' "$SERVICES" |
        sed 's/:.*//' | sort -u
)"

# Homepage links that must be Caddy-served: https, in-zone, no port.
homepage_proxied="$(
    sed -n 's|^[[:space:]]*href:[[:space:]]*https://\([^/:[:space:]]*\)\(/.*\)\{0,1\}$|\1|p' "$SERVICES" |
        grep -F ".${ZONE}" | sort -u
)"

status=0

# check_subset <label> <defined-set> <items> <message>: every item must be in
# the set; <message> describes what a missing item means.
check_subset() {
    local label="$1" defined="$2" items="$3" message="$4" item
    while IFS= read -r item; do
        [ -n "$item" ] || continue
        if ! printf '%s\n' "$defined" | grep -qFx "$item"; then
            echo "ERROR: ${label}: '${item}' ${message}" >&2
            status=1
        fi
    done <<< "$items"
}

# 1. Caddy -> Homepage, minus the endpoints that have no UI.
caddy_ui_hosts="$(
    printf '%s\n' "$caddy_hosts" |
        grep -vxF -f <(printf '%s\n' "$NO_UI_HOSTS") || true
)"
check_subset "caddy.yaml" "$homepage_hosts" "$caddy_ui_hosts" \
    "is published by Caddy but has no entry in k8s/homepage/chart/config/services.yaml (add it, or list it in NO_UI_HOSTS if it has no UI)"

# 2. Homepage -> Caddy.
check_subset "services.yaml" "$caddy_hosts" "$homepage_proxied" \
    "is linked from the dashboard but is not a Caddy site in ansible/inventories/homelab/group_vars/caddy.yaml (dead link)"

if [ "$status" -ne 0 ]; then
    echo "Caddy sites and the Homepage dashboard are out of sync. Fix the entries above." >&2
    exit 1
fi
echo "OK: Caddy sites and the Homepage dashboard agree."
