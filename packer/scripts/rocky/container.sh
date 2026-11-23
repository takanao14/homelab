#!/bin/bash
set -euo pipefail

echo "Installing Podman container runtime..."

dnf install -y podman podman-docker
