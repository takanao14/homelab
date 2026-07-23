#!/bin/bash

set -euo pipefail

# Can be overridden via the TIMEZONE environment variable.
TIMEZONE="${TIMEZONE:-Asia/Tokyo}"

timedatectl set-timezone "${TIMEZONE}"
