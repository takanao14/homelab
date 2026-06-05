#!/usr/bin/env bash
set -e

# 対象のユーザーを特定する (sudoで実行された場合は元のユーザー名を取得)
TARGET_USER="${SUDO_USER:-${USER}}"

echo "Updating package list..."
sudo apt-get update -q

echo "Installing qemu/kvm and related packages..."
sudo apt-get install -y -q qemu-kvm libvirt-daemon-system libvirt-clients bridge-utils virt-manager libguestfs-tools

echo "Adding user '${TARGET_USER}' to the libvirt group..."
sudo usermod -aG kvm "${TARGET_USER}"
sudo usermod -aG libvirt "${TARGET_USER}"

echo ""
echo "✅ Installation complete!"
echo "⚠️  Note: Group changes may not take effect immediately."
echo "Please log out and log back in, or run 'newgrp libvirt' to apply the new group permissions."
