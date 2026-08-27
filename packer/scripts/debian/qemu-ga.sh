#!/bin/bash
set -euo pipefail

echo "Installing QEMU Guest Agent..."

apt-get update

# Enables features like coordinated snapshots and graceful shutdowns.
apt-get install -y qemu-guest-agent
systemctl enable qemu-guest-agent
