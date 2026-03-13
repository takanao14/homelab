# Node Exporter Role

Installs and configures `prometheus-node-exporter` on Debian-based systems.

## Functionality
- Installs the `prometheus-node-exporter` package from APT repositories.
- Installs `lm-sensors` on x86_64 architectures to enable temperature metric collection.
- Configures command-line arguments for the `node_exporter` service via `/etc/default/prometheus-node-exporter`.
- Ensures the `prometheus-node-exporter` service is started and enabled on boot.

## Variables

### `node_exporter_args`
A list of command-line arguments to pass to the `node_exporter` service. This is useful for enabling or disabling specific collectors.

**Default:** `[]` (empty list)

**Example:**
This variable is typically defined in the inventory file (`inventories/homelab/hosts.yml`) to apply host-specific settings.

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
This role is typically used within a playbook targeting hosts that require monitoring.

```yaml
# In a playbook, e.g., playbooks/node_exporter.yml
- name: Install and configure prometheus-node-exporter
  hosts: node_exporter
  become: yes
  roles:
    - node_exporter
```
