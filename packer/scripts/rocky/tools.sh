#!/bin/bash
# Desktop and development tools installation script
# Installs Chrome, Wireshark, VS Code, and HashiCorp tools
# These tools are commonly used for development and system administration

set -euo pipefail

echo "Installing desktop and development tools..."

# Update package lists
dnf update -y

# Install Google Chrome browser
# Download and install the latest stable version
curl -sS -L -o google-chrome-stable_current_x86_64.rpm https://dl.google.com/linux/direct/google-chrome-stable_current_x86_64.rpm
dnf install -y ./google-chrome-stable_current_x86_64.rpm
rm -f google-chrome-stable_current_x86_64.rpm

# Install Wireshark network protocol analyzer
dnf install -y wireshark

# Install Visual Studio Code
# Reference: https://code.visualstudio.com/docs/setup/linux

# Add Microsoft RPM repository for VS Code
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

# Install VS Code
dnf update -y
dnf install -y code

# Install HashiCorp tools (Terraform, Packer, Vault)
# Reference: https://developer.hashicorp.com/terraform/install
yum install -y yum-utils
sudo yum-config-manager --add-repo https://rpm.releases.hashicorp.com/RHEL/hashicorp.repo

# Install Terraform, Packer, and Vault
dnf update -y
dnf install -y terraform packer vault
