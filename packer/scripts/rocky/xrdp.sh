#!/bin/bash
set -euo pipefail

echo "Installing XRDP desktop environment with XFCE and Japanese support..."

LOCALE="${LOCALE:-ja_JP.UTF-8}"

dnf install -y epel-release
# Enable CRB to satisfy XRDP/Xfce dependencies on Rocky
dnf install -y dnf-plugins-core
dnf config-manager --set-enabled crb

dnf clean all
dnf update -y
dnf upgrade -y

dnf install -y bash-completion
dnf groupinstall -y "Xfce"
dnf install -y xrdp xorgxrdp
dnf install -y langpacks-ja glibc-langpack-ja
dnf install -y ibus ibus-anthy
dnf install -y google-noto-sans-cjk-jp-fonts

localectl set-locale LANG="${LOCALE}"

# XFCE session startup with IBus wired in as the input method.
mv /usr/libexec/xrdp/startwm.sh /usr/libexec/xrdp/startwm.sh.bak
cat > /usr/libexec/xrdp/startwm.sh << EOF
#!/bin/sh
export GTK_IM_MODULE=ibus
export QT_IM_MODULE=ibus
export XMODIFIERS=@im=ibus
ibus-daemon -drx

if [ -x /usr/bin/startxfce4 ]; then
    exec /usr/bin/startxfce4
fi
EOF

chmod +x /usr/libexec/xrdp/startwm.sh

systemctl enable xrdp
systemctl restart xrdp
