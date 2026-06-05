#!/bin/bash
# XRDP and XFCE desktop environment installation script
# Installs XRDP remote desktop server with XFCE desktop environment
# Includes Japanese language support and Fcitx5 input method

set -euo pipefail

echo "Installing XRDP desktop environment with XFCE and Japanese support..."

# Set locale
LOCALE="${LOCALE:-ja_JP.UTF-8}"

# Install EPEL repository for additional packages
dnf install -y epel-release
# Enable CRB to satisfy XRDP/Xfce dependencies on Rocky
dnf install -y dnf-plugins-core
dnf config-manager --set-enabled crb

# Update package lists
dnf clean all
dnf update -y
dnf upgrade -y

# Install bash-completion for improved command-line experience
dnf install -y bash-completion

# Install the XFCE desktop environment group
dnf groupinstall -y "Xfce"

# Install XRDP server and Xorg backend
dnf install -y xrdp xorgxrdp

# Install Japanese language packs
dnf install -y langpacks-ja glibc-langpack-ja

# Install IBus with Anthy for Japanese input
dnf install -y ibus ibus-anthy

# Install Japanese CJK fonts
dnf install -y google-noto-sans-cjk-jp-fonts

# Persist locale configuration
localectl set-locale LANG="${LOCALE}"

# Replace XRDP session startup to launch IBus and XFCE
mv /usr/libexec/xrdp/startwm.sh /usr/libexec/xrdp/startwm.sh.bak
cat > /usr/libexec/xrdp/startwm.sh << EOF
#!/bin/sh
export GTK_IM_MODULE=ibus
export QT_IM_MODULE=ibus
export XMODIFIERS=@im=ibus
ibus-daemon -drx  # IBusをバックグラウンドで起動

if [ -x /usr/bin/startxfce4 ]; then
    exec /usr/bin/startxfce4
fi
EOF

chmod +x /usr/libexec/xrdp/startwm.sh

# Enable and start XRDP service
systemctl enable xrdp
systemctl restart xrdp
