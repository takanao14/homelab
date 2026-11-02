#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -n "${IMAGEBUILDER_BINARY:-}" ]]; then
    exec "$IMAGEBUILDER_BINARY" controller "$@"
fi
(
    unset GOOS GOARCH
    bash "${SCRIPT_DIR}/imagebuilder/build.sh"
)
exec "${SCRIPT_DIR}/imagebuilder/bin/imagebuilder" controller "$@"
