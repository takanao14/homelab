#!/usr/bin/env bash
set -euo pipefail

# Install QEMU/KVM and libguestfs once on an Ubuntu/Debian builder.

# Resolve the target user (the invoking user when run via sudo).
TARGET_USER="${SUDO_USER:-${USER}}"

echo "Updating package list..."
sudo apt-get update -q

echo "Installing qemu/kvm and related packages..."
sudo apt-get install -y -q qemu-kvm libvirt-daemon-system libvirt-clients bridge-utils virt-manager libguestfs-tools

echo "Adding user '${TARGET_USER}' to the kvm and libvirt groups..."
sudo usermod -aG kvm "${TARGET_USER}"
sudo usermod -aG libvirt "${TARGET_USER}"

echo ""
echo "✅ Installation complete!"
echo "⚠️  Note: Group changes may not take effect immediately."
echo "Please log out and log back in, or run 'newgrp libvirt' to apply the new group permissions."
