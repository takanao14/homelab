#!/bin/bash
# Kubernetes tools installation script
# Installs kubectl and Helm

set -euo pipefail

echo "Installing Kubernetes tools..."

# Update package lists
dnf update -y

# Install kubectl from official Kubernetes repository
# Reference: https://kubernetes.io/docs/tasks/tools/install-kubectl-linux/#install-using-native-package-management

# Install required dependencies
dnf install -y curl gnupg

# Set up Kubernetes RPM repository
cat > /etc/yum.repos.d/kubernetes.repo << EOF
[kubernetes]
name=Kubernetes
baseurl=https://pkgs.k8s.io/core:/stable:/v1.35/rpm/
enabled=1
gpgcheck=1
gpgkey=https://pkgs.k8s.io/core:/stable:/v1.35/rpm/repodata/repomd.xml.key
EOF

# Update package lists and install kubectl
dnf update -y
dnf install -y kubectl

# Install Helm - Kubernetes package manager
# Reference: https://helm.sh/docs/intro/install/
dnf install -y helm
