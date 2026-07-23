#!/bin/bash
set -euo pipefail

echo "Installing QEMU Guest Agent..."

# Can be overridden via the TIMEZONE environment variable.
TIMEZONE="${TIMEZONE:-Asia/Tokyo}"

apt-get update

# Enables features like coordinated snapshots and graceful shutdowns.
apt-get install -y qemu-guest-agent
systemctl enable qemu-guest-agent

timedatectl set-timezone "${TIMEZONE}"
