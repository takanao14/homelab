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

Command-line arguments are combined from three layers (all default to `[]`):

| Variable | Scope | Defined in |
|----------|-------|------------|
| `node_exporter_base_args` | All hosts | `group_vars/node_exporter.yaml` |
| `node_exporter_rpi_args` | Raspberry Pi hosts | `group_vars/node_exporter_rpi.yaml` |
| `node_exporter_lxc_args` | LXC guests | `group_vars/node_exporter_lxc.yaml` |

The LXC layer disables hardware collectors (`thermal_zone`, `hwmon`):
LXC guests share the host kernel and would otherwise re-report the host's
sensors under their own instance name, duplicating temperature panels and
hardware alerts. Beware: an unknown `--no-collector.*` name makes
node_exporter exit at startup, so only use names from
`prometheus-node-exporter --help`.

## Dependencies

None.

## Usage

```yaml
# In playbooks/common-node_exporter.yaml
- name: Install and configure prometheus-node-exporter
  hosts: node_exporter
  roles:
    - node_exporter
```
