# Node Exporter Role

Installs and configures `prometheus-node-exporter` on Debian-based systems.

## Functionality

- Installs `prometheus-node-exporter` from APT.
- Installs `lm-sensors` on x86_64 for temperature metric collection.
- Configures command-line arguments via `/etc/default/prometheus-node-exporter`.
- Sets up the textfile collector directory (`/var/lib/node_exporter/textfile_collector`).
- On ARM hosts (Raspberry Pi), installs a throttling metrics script and a cron job to collect it.
- Ensures the service is started and enabled.

## Variables

### `node_exporter_args`

A list of command-line arguments passed to `node_exporter`. Defined per-host in the inventory.

**Default:** `[]`

```yaml
# In inventories/homelab/hosts.yml
node_exporter:
  hosts:
    pve:
      node_exporter_args:
        - '--no-collector.schedstat'
        - '--no-collector.netstat'
        - '--no-collector.systemd'
    rpi4:
      node_exporter_args:
        - '--no-collector.btrfs'
        - '--no-collector.edac'
```

## Dependencies

None.

## Usage

```yaml
# In playbooks/node_exporter.yml
- name: Install and configure prometheus-node-exporter
  hosts: node_exporter
  roles:
    - node_exporter
```
