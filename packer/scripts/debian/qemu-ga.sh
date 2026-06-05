#!/bin/bash

# QEMU Guest Agent installation and basic system configuration
# This script installs QEMU Guest Agent for improved VM management
# and sets the system timezone

set -euo pipefail

echo "Installing QEMU Guest Agent..."

# Set timezone (can be overridden via TIMEZONE environment variable)
# Default: Asia/Tokyo
TIMEZONE="${TIMEZONE:-Asia/Tokyo}"

# Update package lists
apt-get update

# Install QEMU Guest Agent for enhanced VM integration
# Enables features like coordinated snapshots and graceful shutdowns
apt-get install -y qemu-guest-agent
systemctl enable qemu-guest-agent

# Configure system timezone
timedatectl set-timezone "${TIMEZONE}"
