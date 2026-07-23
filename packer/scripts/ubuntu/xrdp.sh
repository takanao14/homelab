#!/bin/bash
set -euo pipefail

echo "Installing XRDP desktop environment with XFCE and Japanese support..."

LOCALE="${LOCALE:-ja_JP.UTF-8}"

apt-get update
apt-get install -y language-pack-ja language-pack-ja-base

apt-get install -y xfce4 xfce4-goodies
apt-get install -y xrdp
apt-get install -y fcitx5 fcitx5-mozc fcitx5-config-qt
apt-get install -y fonts-noto-cjk fonts-noto-cjk-extra

locale-gen "${LOCALE}"
adduser xrdp ssl-cert

# XFCE session startup with Fcitx5 wired in as the input method.
cat > /etc/xrdp/startwm.sh << EOF
#!/bin/sh
export LANG="${LOCALE}"
export GTK_IM_MODULE=fcitx
export QT_IM_MODULE=fcitx
export XMODIFIERS=@im=fcitx

# Ensure XAUTHORITY is set and writable for X11 auth.
if [ -z "\$XAUTHORITY" ]; then
    export XAUTHORITY=\$HOME/.Xauthority
fi
if [ ! -f "\$XAUTHORITY" ] || [ ! -w "\$XAUTHORITY" ]; then
    rm -f "\$XAUTHORITY"
    touch "\$XAUTHORITY"
    chmod 600 "\$XAUTHORITY"
fi

(sleep 2; fcitx5 -d) &

exec startxfce4
EOF

chmod +x /etc/xrdp/startwm.sh

# Also needed system-wide (e.g. for non-session processes).
if ! grep -q "GTK_IM_MODULE=fcitx" /etc/environment; then
cat >> /etc/environment << 'EOF'
GTK_IM_MODULE=fcitx
QT_IM_MODULE=fcitx
XMODIFIERS=@im=fcitx
EOF
fi

systemctl enable xrdp
systemctl restart xrdp
