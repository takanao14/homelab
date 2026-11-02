#!/bin/bash
set -euo pipefail

echo "Configuring GNOME Remote Desktop remote login..."

systemctl set-default graphical.target
systemctl enable gnome-remote-desktop.service

if command -v firewall-offline-cmd > /dev/null 2>&1; then
    firewall-offline-cmd --add-service=rdp
fi

# Every VM cloned from the image needs its own TLS key, so it is generated on
# first boot instead of at build time. The RDP credentials are a secret and are
# set per host after provisioning (see packer/README.md).
install -d -m 0755 /usr/local/libexec
cat >/usr/local/libexec/grd-system-init << 'EOF'
#!/bin/bash
set -euo pipefail

home="$(getent passwd gnome-remote-desktop | cut -d: -f6)"
[ -f "${home}/rdp-tls.crt" ] && exit 0

runuser -u gnome-remote-desktop -- openssl req -x509 -newkey rsa:4096 -nodes \
    -sha256 -days 3650 -subj "/CN=$(hostname -f)" \
    -keyout "${home}/rdp-tls.key" -out "${home}/rdp-tls.crt"
grdctl --system rdp set-tls-key "${home}/rdp-tls.key"
grdctl --system rdp set-tls-cert "${home}/rdp-tls.crt"
grdctl --system rdp enable
# The daemon starts before this unit and does not start the RDP server on enable.
systemctl restart gnome-remote-desktop.service
EOF
chmod 0755 /usr/local/libexec/grd-system-init

cat > /etc/systemd/system/grd-system-init.service << 'EOF'
[Unit]
Description=Generate the GNOME Remote Desktop TLS key and enable remote login
Wants=gnome-remote-desktop.service
After=cloud-init.target gnome-remote-desktop.service

[Service]
Type=oneshot
ExecStart=/usr/local/libexec/grd-system-init

[Install]
WantedBy=graphical.target
EOF

systemctl enable grd-system-init.service
