#!/usr/bin/env bash
set -euo pipefail

# Sync the reviewable, vendored powerdns-webui app.
#
# Usage:
#   sync.sh            Fetch the ref recorded in REVISION and overwrite index.html.
#   sync.sh --check    Check the vendored copy for drift.
#   REF=<tag> sync.sh  Fetch a different ref and record it.
#
# Renovate ref bumps fail CI until the vendored bytes are refreshed.

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

# Default to the Renovate-managed recorded ref.
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

# Reject new external resource loads that could exfiltrate zone data.
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
# Vendored from ${REPO}; update with sync.sh, not by hand.
repo: ${REPO}
# renovate: datasource=github-tags depName=james-stevens/powerdns-webui
ref: ${REF}
sha256: ${sha}
date: $(date -u +%Y-%m-%dT%H:%M:%SZ)
EOF

echo "Synced index.html from ${REPO}@${REF} (sha256 ${sha})."
echo "Re-verify the read-only behaviour before merging (see k8s/pdns-ui/README.md)."
