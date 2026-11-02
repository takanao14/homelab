#!/usr/bin/env bash
set -euo pipefail

SOURCE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUTPUT="${1:-${SOURCE_DIR}/bin/imagebuilder}"
GO_VERSION="$(awk '$1 == "go" {print $2}' "${SOURCE_DIR}/go.mod")"
mkdir -p "$(dirname "$OUTPUT")"
cd "$SOURCE_DIR"
# Tooling diagnostics may include credentials from inherited download settings.
# shellcheck disable=SC2034
if ! build_output=$(GOTOOLCHAIN="go${GO_VERSION}" CGO_ENABLED=0 go build \
    -trimpath -buildvcs=false -ldflags='-s -w -buildid=' -o "$OUTPUT" . 2>&1); then
    unset build_output
    echo 'Error: imagebuilder Go build failed.' >&2
    exit 1
fi
unset build_output
