#!/bin/bash

set -euo pipefail

echo "Setting the system timezone..."

TIMEZONE="${TIMEZONE:-Asia/Tokyo}"

timedatectl set-timezone "${TIMEZONE}"
