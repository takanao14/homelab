#!/bin/bash
set -euo pipefail

echo "Installing Podman container runtime..."

apt-get update
apt-get install -y podman podman-docker
