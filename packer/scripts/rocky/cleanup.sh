#!/bin/bash
set -euo pipefail

echo "Cleaning up the system image..."

dnf autoremove -y
dnf clean all
rm -rf /var/cache/dnf/*

rm -f /etc/NetworkManager/system-connections/*.nmconnection

cloud-init clean --logs --seed

find /var/log -type f -exec truncate -s 0 {} \;
rm -rf /var/log/journal/*

rm -rf /tmp/*
rm -rf /var/tmp/*

rm -f /var/lib/systemd/random-seed

truncate -s 0 /etc/machine-id

sync
userdel -r -f rocky || true
