#!/bin/bash
set -euo pipefail

echo "Installing GNOME desktop with Japanese support..."

LOCALE="${LOCALE:-ja_JP.UTF-8}"

apt-get update
# The trailing "firefox-" keeps the snap transitional package out; tools.sh
# installs Firefox ESR from Mozilla's repository instead.
apt-get install -y ubuntu-desktop-minimal gnome-remote-desktop firefox-
apt-get install -y language-pack-ja language-pack-gnome-ja
apt-get install -y ibus-mozc fonts-noto-cjk fonts-noto-cjk-extra

locale-gen "${LOCALE}"
update-locale LANG="${LOCALE}"
