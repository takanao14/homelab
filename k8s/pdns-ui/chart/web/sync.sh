#!/usr/bin/env bash
set -euo pipefail

# Sync the vendored powerdns-webui single-page app.
#
# The app is vendored instead of fetched at runtime so the third-party
# JavaScript that reads the homelab zone data is reviewable in git. This script
# is the only place that talks to GitHub; run it to refresh the local copy.
#
# Usage:
#   sync.sh            Fetch the ref recorded in REVISION and overwrite index.html.
#   sync.sh --check    Fetch into a temp dir and diff against the vendored copy.
#                      Exits non-zero on drift (used by CI).
#   REF=<tag> sync.sh  Fetch a different ref and record it.
#
# Renovate bumps the ref in REVISION; CI then fails until this script is re-run,
# so the recorded version and the vendored bytes cannot drift apart.

REPO="${REPO:-james-stevens/powerdns-webui}"
SRC_PATH="htdocs/index.html"

VENDOR_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REVISION_FILE="${VENDOR_DIR}/REVISION"
TARGET="${VENDOR_DIR}/index.html"

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d' ' -f1
  else
    shasum -a 256 "$1" | cut -d' ' -f1
  fi
}

field_of() {
  sed -n "s/^$1:[[:space:]]*//p" "$REVISION_FILE"
}

# Default to the ref already recorded, so a Renovate bump of REVISION drives
# the next fetch.
recorded_ref=""
[[ -f "$REVISION_FILE" ]] && recorded_ref="$(field_of ref)"
REF="${REF:-${recorded_ref}}"
if [[ -z "$REF" ]]; then
  echo "Error: no REF given and none recorded in ${REVISION_FILE}" >&2
  exit 1
fi

CHECK=0
[[ "${1:-}" == "--check" ]] && CHECK=1

tmp_dir="$(mktemp -d)"
cleanup() { rm -rf "$tmp_dir"; }
trap cleanup EXIT

curl -fsSL "https://raw.githubusercontent.com/${REPO}/${REF}/${SRC_PATH}" \
  -o "${tmp_dir}/index.html"

# The app is served same-origin with an nginx proxy that injects a PowerDNS API
# key, so any external resource load would be a new path for zone data to leave
# the network. Refuse to vendor a version that gained one.
if grep -qE '(src|href)="https?://' "${tmp_dir}/index.html"; then
  echo "Error: ${REPO}@${REF} loads external resources; review it before vendoring." >&2
  exit 1
fi

sha="$(sha256_of "${tmp_dir}/index.html")"

if [[ "$CHECK" -eq 1 ]]; then
  drift=0
  if ! diff -q "$TARGET" "${tmp_dir}/index.html" >/dev/null 2>&1; then
    echo "DRIFT: index.html differs from ${REPO}@${REF}" >&2
    drift=1
  fi
  recorded_sha="$(field_of sha256)"
  if [[ "$recorded_sha" != "$sha" ]]; then
    echo "DRIFT: REVISION records sha256 ${recorded_sha}, upstream ${REF} is ${sha}" >&2
    drift=1
  fi
  if [[ "$drift" -eq 1 ]]; then
    echo "Run k8s/pdns-ui/chart/web/sync.sh, then re-verify the read-only behaviour" >&2
    echo "documented in k8s/pdns-ui/README.md before merging." >&2
    exit 1
  fi
  echo "Vendored index.html is in sync with ${REPO}@${REF}."
  exit 0
fi

install -m 0644 "${tmp_dir}/index.html" "$TARGET"

cat > "$REVISION_FILE" <<EOF
# Vendored from ${REPO}, synced by sync.sh. Do not edit index.html by hand;
# re-run sync.sh to update it. Bumping the ref alone fails CI until the
# vendored bytes are refreshed.
repo: ${REPO}
# renovate: datasource=github-tags depName=james-stevens/powerdns-webui
ref: ${REF}
sha256: ${sha}
date: $(date -u +%Y-%m-%dT%H:%M:%SZ)
EOF

echo "Synced index.html from ${REPO}@${REF} (sha256 ${sha})."
echo "Re-verify the read-only behaviour before merging (see k8s/pdns-ui/README.md)."
