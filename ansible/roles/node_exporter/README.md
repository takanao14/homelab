# Node Exporter Role

Installs and configures `prometheus-node-exporter` on Debian-based systems.

## Functionality

- Installs `prometheus-node-exporter` from APT.
- Installs `lm-sensors` on x86_64 for temperature metric collection, unless
  `node_exporter_install_lm_sensors` is false.
- Configures command-line arguments via `/etc/default/prometheus-node-exporter`.
- Sets up the textfile collector directory (`/var/lib/node_exporter/textfile_collector`).
- Supports service-specific collector flags through `node_exporter_extra_args`.
- On ARM hosts (Raspberry Pi), installs a throttling metrics script and a cron job to collect it.
- Ensures the service is started and enabled.
- Defers service lifecycle checks on a pristine host during Ansible check mode;
  APT does not create the systemd unit until a normal run installs the package.

## Variables

Command-line arguments are combined from four layers (all default to `[]`):

| Variable | Scope | Defined in |
|----------|-------|------------|
| `node_exporter_base_args` | All hosts | `group_vars/node_exporter.yaml` |
| `node_exporter_rpi_args` | Raspberry Pi hosts | `group_vars/node_exporter_rpi.yaml` |
| `node_exporter_lxc_args` | LXC guests | `group_vars/node_exporter_lxc.yaml` |
| `node_exporter_extra_args` | Service-specific hosts | Service group variables |

The LXC layer disables hardware collectors (`thermal_zone`, `hwmon`):
LXC guests share the host kernel and would otherwise re-report the host's
sensors under their own instance name, duplicating temperature panels and
hardware alerts. Beware: an unknown `--no-collector.*` name makes
node_exporter exit at startup, so only use names from
`prometheus-node-exporter --help`.

### Package installation

| Variable | Default | Purpose |
|----------|---------|---------|
| `node_exporter_install_lm_sensors` | `true` | Install `lm-sensors` on x86_64 hosts |

Set to `false` where the `hwmon` collector is disabled anyway, which leaves
`lm-sensors` without a consumer. LXC guests are the case in point: they have no
hardware sensors of their own, and the collector reading the host's sensors is
already turned off.

When the flag is `false` the role also removes `lm-sensors` with `autoremove`,
so hosts provisioned before the flag existed converge on the next run. The
`autoremove` in `playbooks/tasks/package_upgrade.yaml` does not reclaim it on
its own — the role installs the package by name, which marks it manual.

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
