#!/bin/bash
# XRDP and XFCE desktop environment installation script
# Installs XRDP remote desktop server with XFCE desktop environment
# Includes Japanese language support and Fcitx5 input method

set -euo pipefail

echo "Installing XRDP desktop environment with XFCE and Japanese support..."

# Set locale
LOCALE="${LOCALE:-ja_JP.UTF-8}"

# Update package lists
apt-get update
# Install language pack and base system for the configured locale
apt-get install -y language-pack-ja language-pack-ja-base

# Install XFCE4 desktop environment and additional goodies
apt-get install -y xfce4 xfce4-goodies

# Install XRDP remote desktop protocol server
apt-get install -y xrdp

# Install Fcitx5 input method framework with Mozc (Japanese input)
apt-get install -y fcitx5 fcitx5-mozc fcitx5-config-qt

# Install Noto CJK fonts for proper Asian character rendering
apt-get install -y fonts-noto-cjk fonts-noto-cjk-extra

# Generate locale and add xrdp user to ssl-cert group
locale-gen "${LOCALE}"
adduser xrdp ssl-cert

# Create custom XRDP startup script
# This script configures the environment and starts XFCE4 with Fcitx5
cat > /etc/xrdp/startwm.sh << EOF
#!/bin/sh
# Set locale and input method environment variables
export LANG="${LOCALE}"
export GTK_IM_MODULE=fcitx
export QT_IM_MODULE=fcitx
export XMODIFIERS=@im=fcitx

# Configure .Xauthority file for X11 authentication
# Ensure XAUTHORITY is set and writable
if [ -z "\$XAUTHORITY" ]; then
    export XAUTHORITY=\$HOME/.Xauthority
fi
if [ ! -f "\$XAUTHORITY" ] || [ ! -w "\$XAUTHORITY" ]; then
    rm -f "\$XAUTHORITY"
    touch "\$XAUTHORITY"
    chmod 600 "\$XAUTHORITY"
fi

# Start Fcitx5 input method daemon in background
(sleep 2; fcitx5 -d) &

# Start XFCE4 desktop environment
exec startxfce4
EOF

# Make the startup script executable
chmod +x /etc/xrdp/startwm.sh

# Add input method environment variables system-wide
if ! grep -q "GTK_IM_MODULE=fcitx" /etc/environment; then
cat >> /etc/environment << 'EOF'
GTK_IM_MODULE=fcitx
QT_IM_MODULE=fcitx
XMODIFIERS=@im=fcitx
EOF
fi

# Enable and start XRDP service
systemctl enable xrdp
systemctl restart xrdp
