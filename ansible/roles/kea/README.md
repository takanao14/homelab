# kea Role

Installs and configures [Kea DHCPv4](https://www.isc.org/kea/) server on Debian-based systems.

## Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `kea_subnet4` | `[]` | List of DHCPv4 subnet configurations |

Define `kea_subnet4` in `group_vars/dhcp.yaml` (or `group_vars/dhcp.sops.yaml`);
the role renders the list directly into the Kea config template.

Example:

```yaml
kea_subnet4:
  - subnet: "192.168.10.0/24"
    pool: "192.168.10.100 - 192.168.10.200"
    routers: "192.168.10.1"
    dns_servers: "192.168.10.237, 192.168.10.238"
    domain_name: "home.butaco.net"
    reservations:
      - hw_address: "aa:bb:cc:dd:ee:ff"
        ip_address: "192.168.10.10"
```

## Placement

DHCP runs on rpi4, outside the Proxmox / cluster nodes, so that AMT out-of-band
management of `node1`–`node3` keeps working during node outages. See
[ADR-0002](../../../docs/adr/0002-dhcp-outside-proxmox-cluster-nodes.md).

## Usage

Run [playbooks/services/dhcp.yaml](../../playbooks/services/dhcp.yaml).
