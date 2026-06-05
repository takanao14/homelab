#!/bin/bash
# Desktop and development tools installation script

set -euo pipefail

echo "Installing desktop and development tools..."

# Update package lists
apt-get update

# Install Google Chrome browser
wget -q https://dl.google.com/linux/direct/google-chrome-stable_current_amd64.deb
dpkg -i google-chrome-stable_current_amd64.deb || apt-get -f install -y
rm -f google-chrome-stable_current_amd64.deb

# Install Wireshark network protocol analyzer
DEBIAN_FRONTEND=noninteractive apt-get install -y wireshark

# Install Visual Studio Code
# Reference: https://code.visualstudio.com/docs/setup/linux#_install-vs-code-on-linux

echo "code code/add-microsoft-repo boolean true" | debconf-set-selections

# Install dependencies and add Microsoft GPG key
apt-get install -y wget gpg
wget -qO- https://packages.microsoft.com/keys/microsoft.asc | gpg --dearmor > microsoft.gpg
install -D -o root -g root -m 644 microsoft.gpg /usr/share/keyrings/microsoft.gpg
rm -f microsoft.gpg

# Configure VS Code repository using DEB822 format
cat > /etc/apt/sources.list.d/vscode.sources << 'EOF'
Types: deb
URIs: https://packages.microsoft.com/repos/code
Suites: stable
Components: main
Architectures: amd64,arm64,armhf
Signed-By: /usr/share/keyrings/microsoft.gpg
EOF

# Install VS Code from Microsoft repository
apt-get install -y apt-transport-https
apt-get update
apt-get install -y code

# Install HashiCorp tools (Terraform, Packer, Vault)
# Reference: https://developer.hashicorp.com/terraform/install

# Install prerequisites
apt-get install -y gnupg software-properties-common

# Add HashiCorp GPG key and repository
wget -O- https://apt.releases.hashicorp.com/gpg | gpg --dearmor | tee /usr/share/keyrings/hashicorp-archive-keyring.gpg > /dev/null

# Source os-release to get the codename reliably
. /etc/os-release
echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com ${UBUNTU_CODENAME:-$(lsb_release -cs)} main" | tee /etc/apt/sources.list.d/hashicorp.list > /dev/null

# Install Terraform, Packer, and Vault
apt-get update
apt-get install -y terraform packer vault
