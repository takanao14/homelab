#!/bin/bash
# Container runtime installation script
# Installs Podman as a Docker-compatible container runtime

set -euo pipefail

echo "Installing Podman container runtime..."

# Update package lists
apt-get update

# Install Podman and Docker compatibility layer
apt-get install -y podman podman-docker
