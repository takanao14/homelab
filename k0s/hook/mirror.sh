#!/bin/bash
set -euo pipefail

# Color output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Configuration
CONTAINERD_DIR="/etc/k0s/containerd.d"
CERTS_DIR="/etc/k0s/certs.d/docker.io"
REGISTRY_FILE="${CONTAINERD_DIR}/registry-path.toml"
HOSTS_FILE="${CERTS_DIR}/hosts.toml"
MIRROR_ENDPOINT="https://mirror.gcr.io"

# Pre-flight checks
preflight_checks() {
    echo -e "${YELLOW}→${NC} Checking prerequisites..."

    # Check sudo access
    if ! sudo -n true 2>/dev/null; then
        echo -e "${YELLOW}→${NC} Authenticating with sudo..."
        sudo true
    fi

    echo -e "${GREEN}✓${NC} Prerequisites met"
}

create_directories() {
    local containerd_dir="$1"
    local certs_dir="$2"

    # Create containerd directory
    if [ ! -d "$containerd_dir" ]; then
        echo -e "${YELLOW}→${NC} Creating $containerd_dir..."
        sudo mkdir -p "$containerd_dir"
    else
        echo -e "${GREEN}✓${NC} $containerd_dir exists"
    fi

    # Create certificate directory
    if [ ! -d "$certs_dir" ]; then
        echo -e "${YELLOW}→${NC} Creating $certs_dir..."
        sudo mkdir -p "$certs_dir"
    else
        echo -e "${GREEN}✓${NC} $certs_dir exists"
    fi
}

configure_registry() {
    local registry_file="$1"
    local certs_base_path="/etc/k0s/certs.d"

    # Create registry configuration if not exists
    if [ -f "$registry_file" ]; then
        echo -e "${GREEN}✓${NC} Registry configuration already exists"
    else
        echo -e "${YELLOW}→${NC} Creating registry configuration..."
        sudo tee "$registry_file" > /dev/null <<EOF
version = 2

[plugins."io.containerd.grpc.v1.cri".registry]
  config_path = "$certs_base_path"
EOF
        echo -e "${GREEN}✓${NC} Registry configured at $registry_file"
    fi
}

configure_hosts() {
    local hosts_file="$1"
    local mirror_endpoint="$2"

    # Create hosts configuration if not exists
    if [ -f "$hosts_file" ]; then
        echo -e "${GREEN}✓${NC} docker.io hosts configuration already exists"
    else
        echo -e "${YELLOW}→${NC} Creating docker.io hosts configuration..."
        sudo tee "$hosts_file" > /dev/null <<EOF
server = "https://registry-1.docker.io"

[host."${mirror_endpoint}"]
  capabilities = ["pull", "resolve"]
EOF
        echo -e "${GREEN}✓${NC} docker.io hosts configuration added"
    fi
}

# Main execution
echo -e "${GREEN}=== Mirror Setup Script ===${NC}"
preflight_checks
create_directories "$CONTAINERD_DIR" "$CERTS_DIR"
configure_registry "$REGISTRY_FILE"
configure_hosts "$HOSTS_FILE" "$MIRROR_ENDPOINT"

echo -e "${GREEN}✓${NC} Setup completed successfully"
echo -e "${GREEN}✓${NC} Mirror endpoint: $MIRROR_ENDPOINT"
echo -e "${GREEN}✓${NC} Config directory: $CERTS_DIR"
