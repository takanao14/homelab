#!/bin/bash
set -euo pipefail

echo "Installing desktop and development tools..."

dnf update -y
dnf install -y firefox
dnf install -y wireshark

# https://code.visualstudio.com/docs/setup/linux
rpm --import https://packages.microsoft.com/keys/microsoft.asc
cat > /etc/yum.repos.d/vscode.repo << 'EOF'
[code]
name=Visual Studio Code
baseurl=https://packages.microsoft.com/yumrepos/vscode
enabled=1
autorefresh=1
type=rpm-md
gpgcheck=1
gpgkey=https://packages.microsoft.com/keys/microsoft.asc
EOF

dnf update -y
dnf install -y code

# https://developer.hashicorp.com/terraform/install
# Fetched directly because config-manager syntax differs between dnf4 and dnf5.
curl -fsSL https://rpm.releases.hashicorp.com/RHEL/hashicorp.repo -o /etc/yum.repos.d/hashicorp.repo

dnf update -y
dnf install -y terraform packer vault
