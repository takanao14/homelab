#!/bin/bash

set -euo pipefail

# Set timezone (can be overridden via TIMEZONE environment variable)
# Default: Asia/Tokyo
TIMEZONE="${TIMEZONE:-Asia/Tokyo}"

# Configure system timezone
timedatectl set-timezone "${TIMEZONE}"
