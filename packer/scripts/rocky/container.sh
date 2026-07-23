#!/bin/bash
set -euo pipefail

echo "Installing Podman container runtime..."

dnf update -y
dnf install -y podman podman-docker
