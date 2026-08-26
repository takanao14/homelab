#!/bin/bash
set -euo pipefail

echo "Enabling the EPEL and CRB repositories..."

# The shared toolchain pulls mosh from EPEL, and EPEL packages in turn resolve
# dependencies out of CRB, which epel-release does not enable by itself.
dnf install -y epel-release

# Rocky 10 ships dnf5, whose config-manager plugin and syntax differ from dnf4.
if command -v dnf5 > /dev/null 2>&1; then
    dnf install -y dnf5-plugins
    dnf config-manager setopt crb.enabled=1
else
    dnf install -y dnf-plugins-core
    dnf config-manager --set-enabled crb
fi
