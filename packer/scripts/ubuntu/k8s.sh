#!/bin/bash
# Kubernetes tools installation script
# Installs kubectl and Helm

set -euo pipefail

echo "Installing Kubernetes tools..."

# Update package lists
apt-get update

# Install kubectl from official Kubernetes repository
# Reference: https://kubernetes.io/docs/tasks/tools/install-kubectl-linux/#install-using-native-package-management

# Install required dependencies for APT repository management
apt-get install -y apt-transport-https ca-certificates curl gnupg
# Set up Kubernetes APT repository
mkdir -p -m 755 /etc/apt/keyrings
curl -fsSL https://pkgs.k8s.io/core:/stable:/v1.35/deb/Release.key | gpg --dearmor -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg
chmod 644 /etc/apt/keyrings/kubernetes-apt-keyring.gpg
echo 'deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/v1.35/deb/ /' | tee /etc/apt/sources.list.d/kubernetes.list
chmod 644 /etc/apt/sources.list.d/kubernetes.list

# Update package lists and install kubectl
apt-get update
apt-get install -y kubectl

# Install Helm - Kubernetes package manager
# Reference: https://helm.sh/docs/intro/install/
curl -fsSL https://packages.buildkite.com/helm-linux/helm-debian/gpgkey | gpg --dearmor | tee /usr/share/keyrings/helm.gpg > /dev/null
echo "deb [signed-by=/usr/share/keyrings/helm.gpg] https://packages.buildkite.com/helm-linux/helm-debian/any/ any main" | tee /etc/apt/sources.list.d/helm-stable-debian.list
apt-get update
apt-get install -y helm
