#!/usr/bin/env bash
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${REPO_DIR}/ansible"
exec ansible-playbook playbooks/services/imagebuilder.yaml "$@"
