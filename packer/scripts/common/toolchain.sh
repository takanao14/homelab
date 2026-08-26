#!/usr/bin/env bash
set -euo pipefail

# Install the shared homelab toolchain into an image, system-wide.
#
# Run by image.pkr.hcl after the distro-specific provisioners. The install
# wrappers and their vendored installers are staged by a file provisioner, so
# the build never fetches them from GitHub.
#
# Environment:
#   TOOL_MACHINE_PROFILE  desktop|server; desktop adds Freelens, kitty and the
#                         GUI font (terminal.sh and fonts.sh no-op on server)
#   INSTALL_TOOLCHAIN     "false" skips this step entirely (unsupported distro)
#   INSTALL_DIR           staged copy of scripts/install (default /tmp/install)
#   KITTY_CONF            staged kitty defaults (default /tmp/kitty.conf)

INSTALL_DIR="${INSTALL_DIR:-/tmp/install}"
KITTY_CONF="${KITTY_CONF:-/tmp/kitty.conf}"

if [[ "${INSTALL_TOOLCHAIN:-true}" != "true" ]]; then
  echo "INSTALL_TOOLCHAIN is not true; skipping toolchain installation."
  rm -f "$KITTY_CONF"
  exit 0
fi

export TOOL_MACHINE_PROFILE="${TOOL_MACHINE_PROFILE:-server}"
export VENDOR_DIR="${INSTALL_DIR}/vendor"

echo "Installing the ${TOOL_MACHINE_PROFILE} toolchain system-wide..."

# global mode installs into /usr/local, which outlives the build user that
# cleanup deletes. packages.sh must run first: it provides the OS-level
# dependencies (pipx, python>=3.12) that tools.sh refuses to run without.
bash "${INSTALL_DIR}/packages.sh" global
bash "${INSTALL_DIR}/tools.sh" global
bash "${INSTALL_DIR}/terminal.sh" global
bash "${INSTALL_DIR}/fonts.sh" global

if [[ "$TOOL_MACHINE_PROFILE" == "desktop" && -f "$KITTY_CONF" ]]; then
  echo "Installing system-wide kitty defaults..."
  sudo install -D -m 0644 "$KITTY_CONF" /etc/xdg/kitty/kitty.conf
fi
rm -f "$KITTY_CONF"
