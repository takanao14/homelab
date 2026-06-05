#!/bin/bash
# KVM and virtualization tools installation script
# Installs QEMU/KVM, libvirt, and associated utilities
# for virtual machine management and cloud image handling

set -euo pipefail

echo "Installing KVM/QEMU virtualization tools..."

# Update package lists
dnf update -y

# Install KVM/QEMU virtualization stack
# Includes hypervisor, management tools, and cloud-init utilities
dnf install -y \
    qemu-kvm \
    qemu-img \
    libvirt \
    virt-install \
    virt-manager \
    xorriso \
    libguestfs-tools

# Enable per-subsystem libvirt sockets for on-demand activation
for unit in qemu network storage nodedev nwfilter secret interface; do
    sudo systemctl enable --now virt${unit}d.socket
done
