#!/bin/bash
set -euo pipefail

# Color output for better readability
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Pre-flight checks
preflight_checks() {
    echo -e "${YELLOW}→${NC} Checking prerequisites..."

    # Check if running as appropriate user (should have sudo access)
    if ! sudo -n true 2>/dev/null; then
        echo -e "${YELLOW}→${NC} Script requires sudo access. Authenticating..."
        sudo true
    fi

    # Check required commands
    for cmd in blkid mkfs.ext4 mountpoint; do
        if ! command -v "$cmd" &> /dev/null; then
            echo -e "${RED}✗${NC} Error: Required command '$cmd' not found"
            exit 1
        fi
    done

    echo -e "${GREEN}✓${NC} Prerequisites met"
}

# Check if device exists
device_exists() {
    local device=$1
    if [ ! -b "$device" ]; then
        echo -e "${RED}✗${NC} Error: Device $device does not exist"
        return 1
    fi
}

# Format device with ext4 if not already formatted
format_device() {
    local device=$1

    # Check device exists
    if ! device_exists "$device"; then
        exit 1
    fi

    if sudo blkid -s TYPE -o value "$device" 2>/dev/null | grep -q "^ext4$"; then
        echo -e "${GREEN}✓${NC} $device is already formatted with ext4."
    else
        echo -e "${YELLOW}→${NC} Formatting $device with ext4..."
        sudo mkfs.ext4 -F "$device"
        echo -e "${GREEN}✓${NC} $device formatted successfully"
    fi
}

# Setup mount point and configure fstab
setup_mount() {
    local device=$1
    local mountpoint=$2

    # Check if already mounted
    if mountpoint -q "$mountpoint" 2>/dev/null; then
        echo -e "${GREEN}✓${NC} $mountpoint is already mounted"
    else
        # Create mount directory
        sudo mkdir -p "$mountpoint"

        # Mount device
        sudo mount "$device" "$mountpoint"
        echo -e "${GREEN}✓${NC} $mountpoint mounted successfully"
    fi

    # Get UUID
    local uuid
    if ! uuid=$(sudo blkid -s UUID -o value "$device" 2>/dev/null); then
        echo -e "${RED}✗${NC} Error: Could not retrieve UUID for $device"
        exit 1
    fi

    if [ -z "$uuid" ]; then
        echo -e "${RED}✗${NC} Error: Device $device has no UUID"
        exit 1
    fi

    echo -e "${GREEN}✓${NC} Found UUID for $device: $uuid"

    # Add to fstab if not already present
    if ! grep -q "$mountpoint" /etc/fstab; then
        echo -e "${YELLOW}→${NC} Adding $mountpoint to /etc/fstab"
        echo "UUID=$uuid $mountpoint ext4 defaults 0 2" | sudo tee -a /etc/fstab > /dev/null
    else
        echo -e "${GREEN}✓${NC} $mountpoint already exists in /etc/fstab"
    fi
}

# Main execution
echo -e "${GREEN}=== SSD Setup Script ===${NC}"
preflight_checks

format_device "/dev/sdb"

sudo systemctl daemon-reload

setup_mount /dev/sdb /srv/storage/volume

sudo systemctl daemon-reload

echo -e "${GREEN}✓${NC} SSD setup completed successfully!"
