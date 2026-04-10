# kea Role

Installs and configures [Kea DHCPv4](https://www.isc.org/kea/) server on Debian-based systems.

## Functionality

- Installs `kea-dhcp4-server` and `socat` from APT.
- Ensures `/var/lib/kea` and `/run/kea` directories exist with correct permissions.
- Deploys `/etc/kea/kea-dhcp4.conf` from a Jinja2 template.
- Ensures the `kea-dhcp4-server` service is started and enabled.

## Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `kea_subnet4` | `[]` | List of DHCPv4 subnet configurations |

`kea_subnet4` is passed directly into the Kea config template. Define it in `group_vars/dhcp.yaml`.

Example:

```yaml
kea_subnet4:
  - subnet: "192.168.10.0/24"
    pools:
      - pool: "192.168.10.100 - 192.168.10.200"
    option-data:
      - name: routers
        data: "192.168.10.1"
    reservations:
      - hw-address: "aa:bb:cc:dd:ee:ff"
        ip-address: "192.168.10.10"
        hostname: "myhost"
```

## Dependencies

None.

## Usage

```yaml
- name: Setup DHCP Server
  hosts: dhcp
  roles:
    - role: kea
```
