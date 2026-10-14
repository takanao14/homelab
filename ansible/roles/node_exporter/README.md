# Node Exporter Role

Installs and configures `prometheus-node-exporter` on Debian-based systems.

## Functionality

- Installs `prometheus-node-exporter` from APT.
- Installs `lm-sensors` on x86_64 for temperature metric collection, unless
  `node_exporter_install_lm_sensors` is false.
- Configures command-line arguments via `/etc/default/prometheus-node-exporter`.
- Sets up the textfile collector directory (`/var/lib/prometheus/node-exporter`)
  with a shared writer group and sticky group-writable mode. Only explicitly
  selected service users join the writer group. Use `--tags node_exporter_textfile`
  with `playbooks/common/node_exporter.yaml --limit <host>` to update these permissions alone.
- Supports service-specific collector flags through `node_exporter_extra_args`.
- On ARM hosts (Raspberry Pi), installs a throttling metrics script and a cron job to collect it.
- Masks `openipmi.service`, which APT pulls in behind the exporter (see below).
- Defers service lifecycle checks on a pristine host during Ansible check mode;
  APT does not create the systemd unit until a normal run installs the package.

## Variables

Command-line arguments are combined from five layers (all default to `[]`):

| Variable | Scope | Defined in |
|----------|-------|------------|
| `node_exporter_base_args` | All hosts | `group_vars/node_exporter.yaml` |
| `node_exporter_rpi_args` | Raspberry Pi hosts | `group_vars/node_exporter_rpi.yaml` |
| `node_exporter_lxc_args` | LXC guests | `group_vars/node_exporter_lxc.yaml` |
| `node_exporter_vm_args` | VM guests | `group_vars/node_exporter_vm.yaml` |
| `node_exporter_extra_args` | Service-specific hosts | Service group variables |

Beware: an unknown `--no-collector.*` name makes node_exporter exit at startup,
so only use names from `prometheus-node-exporter --help-long` **on a target
host**. The Debian package trails the k0s node-exporter DaemonSet — `bcachefs`
exists in the latter and not in the former.

### Collector exclusions

Disable collectors only for:

- Unavailable hardware or persistent errors: `fibrechannel`, `infiniband`, and
  `tapestats` fail on this fleet; `rapl` cannot read root-only `energy_uj` files.
- Duplicate guest metrics: LXC shares the host kernel, so `thermal_zone`,
  `hwmon`, `diskstats`, `cpufreq`, `powersupplyclass`, `nvme`, and `zfs` report
  host data under the guest's name. Keep namespaced collectors (`netdev`,
  `netclass`, `filesystem`, `pressure`, `cpu`) enabled. VMs disable only `zfs`;
  their sensor collectors return empty rather than duplicate host data.

Keep collectors enabled when hardware or mounts may appear later, including
`nfs`. Before adding an exclusion, check `node_scrape_collector_success == 0`
or compare the guest's series with its Proxmox host to establish duplication.

### Textfile collector

| Variable | Default | Purpose |
|----------|---------|---------|
| `node_exporter_textfile_group` | `node-exporter-textfile` | Shared group for selected textfile writers |
| `node_exporter_textfile_dir_mode` | `1775` | Sticky group-writable textfile directory mode |

The node_exporter role owns the directory's group and mode. Service roles add
only their dedicated writer users to `node_exporter_textfile_group`; they must
not change the shared directory itself.

### Package installation

| Variable | Default | Purpose |
|----------|---------|---------|
| `node_exporter_install_lm_sensors` | `true` | Install `lm-sensors` on x86_64 hosts |

Set `node_exporter_install_lm_sensors: false` where `hwmon` is disabled,
including LXC guests. The role explicitly removes `lm-sensors` with
`autoremove`: package-upgrade autoremove alone cannot remove a manually
installed package.

### openipmi

| Variable | Default | Purpose |
|----------|---------|---------|
| `node_exporter_mask_openipmi` | `true` | Mask `openipmi.service` |

APT recommendations pull in `openipmi`, whose service fails on this fleet's
hosts without BMCs. Masking survives package reinstalls; removing the collector
package would also remove useful `apt`, `nvme`, and `smartmon` textfile metrics.
The role runs `reset-failed` after masking to clear any recorded failure.

Set to `false` on a host that actually has a BMC.

## Usage

Run [playbooks/common/node_exporter.yaml](../../playbooks/common/node_exporter.yaml).
