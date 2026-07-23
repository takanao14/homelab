#!/bin/bash
set -euo pipefail

echo "Installing KVM/QEMU virtualization tools..."

apt-get update
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
