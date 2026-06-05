#!/bin/bash
# Clean up packages and cloud-init data

set -euo pipefail

echo "Cleaning up the system image..."

# Remove unused packages and clean package cache
dnf autoremove -y
dnf clean all
rm -rf /var/cache/dnf/*

# Clean up NetworkManager connections
rm -f /etc/NetworkManager/system-connections/*.nmconnection

# Clean up cloud-init configuration and data
cloud-init clean --logs --seed

# Clean up log files
find /var/log -type f -exec truncate -s 0 {} \;

# Clean up journal logs
rm -rf /var/log/journal/*

# Clean up temporary files
rm -rf /tmp/*
rm -rf /var/tmp/*

# Clean up random seed
rm -f /var/lib/systemd/random-seed

# Clean up machine-id
truncate -s 0 /etc/machine-id

sync
userdel -r -f rocky || true
