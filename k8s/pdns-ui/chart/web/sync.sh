#!/usr/bin/env bash
set -euo pipefail

# Sync the reviewable, vendored powerdns-webui app.
#
# Usage:
#   sync.sh            Fetch the ref recorded in REVISION and overwrite index.html and LICENSE.
#   sync.sh --check    Check the vendored copy for drift.
#   REF=<tag> sync.sh  Fetch a different ref and record it.
#
# Renovate ref bumps fail CI until the vendored bytes are refreshed.

REPO="${REPO:-james-stevens/powerdns-webui}"

VENDOR_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REVISION_FILE="${VENDOR_DIR}/REVISION"

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

for file in index.html LICENSE; do
  src_path="$file"
  [[ "$file" == "index.html" ]] && src_path="htdocs/index.html"
  curl -fsSL "https://raw.githubusercontent.com/${REPO}/${REF}/${src_path}" \
    -o "${tmp_dir}/${file}"
done

# Reject new external resource loads that could exfiltrate zone data.
if grep -qE '(src|href)="https?://' "${tmp_dir}/index.html"; then
  echo "Error: ${REPO}@${REF} loads external resources; review it before vendoring." >&2
  exit 1
fi

sha="$(sha256_of "${tmp_dir}/index.html")"
license_sha="$(sha256_of "${tmp_dir}/LICENSE")"

if [[ "$CHECK" -eq 1 ]]; then
  drift=0
  for file in index.html LICENSE; do
    if ! diff -q "${VENDOR_DIR}/${file}" "${tmp_dir}/${file}" >/dev/null 2>&1; then
      echo "DRIFT: ${file} differs from ${REPO}@${REF}" >&2
      drift=1
    fi
    sha_field="sha256"
    [[ "$file" == "LICENSE" ]] && sha_field="license_sha256"
    recorded_sha="$(field_of "$sha_field")"
    upstream_sha="$(sha256_of "${tmp_dir}/${file}")"
    if [[ "$recorded_sha" != "$upstream_sha" ]]; then
      echo "DRIFT: ${file} checksum differs from ${sha_field} in REVISION" >&2
      drift=1
    fi
  done
  if [[ "$drift" -eq 1 ]]; then
    echo "Run k8s/pdns-ui/chart/web/sync.sh, then re-verify the read-only behaviour" >&2
    echo "documented in k8s/pdns-ui/README.md before merging." >&2
    exit 1
  fi
  echo "Vendored index.html and LICENSE are in sync with ${REPO}@${REF}."
  exit 0
fi

for file in index.html LICENSE; do
  install -m 0644 "${tmp_dir}/${file}" "${VENDOR_DIR}/${file}"
done

cat > "$REVISION_FILE" <<EOF
# Vendored from ${REPO}; update with sync.sh, not by hand.
repo: ${REPO}
# renovate: datasource=github-tags depName=james-stevens/powerdns-webui
ref: ${REF}
sha256: ${sha}
license_sha256: ${license_sha}
date: $(date -u +%Y-%m-%dT%H:%M:%SZ)
EOF

echo "Synced index.html and LICENSE from ${REPO}@${REF} (sha256 ${sha})."
echo "Re-verify the read-only behaviour before merging (see k8s/pdns-ui/README.md)."
