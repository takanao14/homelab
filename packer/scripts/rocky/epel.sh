#!/bin/bash
set -euo pipefail

echo "Enabling the EPEL and CRB repositories..."

# The shared toolchain pulls mosh from EPEL, which on Rocky 9 resolves
# dependencies out of CRB; epel-release does not enable CRB by itself. Rocky 10
# also ships virt-manager (scripts/rocky/vm.sh) only in CRB.
dnf install -y epel-release

# Rocky 10 ships dnf5, whose config-manager plugin and syntax differ from dnf4.
if command -v dnf5 > /dev/null 2>&1; then
    dnf install -y dnf5-plugins
    dnf config-manager setopt crb.enabled=1
else
    dnf install -y dnf-plugins-core
    dnf config-manager --set-enabled crb
fi
