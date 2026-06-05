#!/bin/bash
# KVM and virtualization tools installation script
# Installs QEMU/KVM, libvirt, and associated utilities
# for virtual machine management and cloud image handling

set -euo pipefail

echo "Installing KVM/QEMU virtualization tools..."

# Update package lists
apt-get update

# Install KVM/QEMU virtualization stack
# Includes hypervisor, management tools, and cloud-init utilities
apt-get install -y \
    qemu-kvm \
    qemu-system-x86 \
    qemu-utils \
    libvirt-daemon-system \
    libvirt-clients \
    bridge-utils \
    virtinst \
    virt-manager \
    cpu-checker \
    cloud-image-utils \
    xorriso \
    libguestfs-tools
