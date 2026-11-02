#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -n "${IMAGEBUILDER_BINARY:-}" ]]; then
    exec "$IMAGEBUILDER_BINARY" verify-remote "$@"
fi
(
    unset GOOS GOARCH
    bash "${SCRIPT_DIR}/../scripts/imagebuilder/build.sh"
)
exec "${SCRIPT_DIR}/../scripts/imagebuilder/bin/imagebuilder" verify-remote "$@"
