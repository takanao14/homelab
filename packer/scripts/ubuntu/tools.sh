#!/bin/bash
# Desktop and development tools installation script

set -euo pipefail

echo "Installing desktop and development tools..."

# Update package lists
apt-get update

# Install Firefox ESR from Mozilla's APT repository.
# Ubuntu 24.04's default "firefox" package is a snap transitional package;
# the Mozilla repo provides a real .deb and avoids snap on this xrdp image.
# ESR is used to match Rocky's firefox package (also ESR) for consistency.
# Reference: https://support.mozilla.org/en-US/kb/install-firefox-linux
apt-get install -y wget
install -d -m 0755 /etc/apt/keyrings
wget -qO- https://packages.mozilla.org/apt/repo-signing-key.gpg | tee /etc/apt/keyrings/packages.mozilla.org.asc > /dev/null
echo "deb [signed-by=/etc/apt/keyrings/packages.mozilla.org.asc] https://packages.mozilla.org/apt mozilla main" > /etc/apt/sources.list.d/mozilla.list

# Pin the Mozilla repo above the Ubuntu snap transitional package
cat > /etc/apt/preferences.d/mozilla << 'EOF'
Package: *
Pin: origin packages.mozilla.org
Pin-Priority: 1000
EOF

apt-get update
apt-get install -y firefox-esr

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
