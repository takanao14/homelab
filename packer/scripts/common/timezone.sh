#!/bin/bash

set -euo pipefail

echo "Setting the system timezone..."

# Can be overridden via the TIMEZONE environment variable.
TIMEZONE="${TIMEZONE:-Asia/Tokyo}"

timedatectl set-timezone "${TIMEZONE}"
