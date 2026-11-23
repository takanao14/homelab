#!/bin/bash
set -euo pipefail

echo "Installing KVM/QEMU virtualization tools..."

dnf install -y \
    qemu-kvm \
    qemu-img \
    libvirt \
    virt-install \
    virt-manager \
    xorriso \
    guestfs-tools

# Enable per-subsystem libvirt sockets for on-demand activation
for unit in qemu network storage nodedev nwfilter secret interface; do
    systemctl enable --now "virt${unit}d.socket"
done
