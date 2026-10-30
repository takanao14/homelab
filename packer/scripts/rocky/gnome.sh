#!/bin/bash
set -euo pipefail

echo "Installing GNOME desktop with Japanese support..."

LOCALE="${LOCALE:-ja_JP.UTF-8}"

dnf update -y
# The Workstation environment already carries gnome-remote-desktop and ibus-anthy.
dnf install -y @^workstation-product-environment
dnf install -y langpacks-ja glibc-langpack-ja google-noto-sans-cjk-jp-fonts

localectl set-locale LANG="${LOCALE}"
