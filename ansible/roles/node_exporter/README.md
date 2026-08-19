# Node Exporter Role

Installs and configures `prometheus-node-exporter` on Debian-based systems.

## Functionality

- Installs `prometheus-node-exporter` from APT.
- Installs `lm-sensors` on x86_64 for temperature metric collection, unless
  `node_exporter_install_lm_sensors` is false.
- Configures command-line arguments via `/etc/default/prometheus-node-exporter`.
- Sets up the textfile collector directory (`/var/lib/prometheus/node-exporter`).
- Supports service-specific collector flags through `node_exporter_extra_args`.
- On ARM hosts (Raspberry Pi), installs a throttling metrics script and a cron job to collect it.
- Masks `openipmi.service`, which APT pulls in behind the exporter (see below).
- Ensures the service is started and enabled.
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

### What earns a `--no-collector.*`

Only two things. A collector that merely has nothing to report *yet* stays
enabled, because it starts reporting on its own once the corresponding mount,
array, or module appears; silencing it would hide the very change worth seeing.
`nfs` is the case in point: it is empty until a host mounts an NFS share, and
`k8s_storage_providers` already carries `nfs` in sandbox.

**Hardware that will never exist on this fleet**, in `node_exporter_base_args`:
`fibrechannel`, `infiniband`, `tapestats`. These fail every scrape rather than
returning empty, so each one also costs a log line per scrape. `rapl` belongs
here for a different reason — it fails on exactly the bare-metal hosts where it
could produce data, because the kernel restricts `energy_uj` to root.

**Host metrics duplicated into a guest.** LXC guests share the host kernel, and
collectors reading paths lxcfs does not virtualize re-report the hypervisor
under the guest's own instance name: `thermal_zone` and `hwmon` (sensors),
`diskstats` (the host's whole block device list), `cpufreq` (the host's physical
core count), `powersupplyclass`, `nvme`, and `zfs`. Collectors that *are*
namespaced — `netdev`, `netclass`, `filesystem`, `pressure`, `cpu` — must stay
enabled. `node_exporter_vm_args` disables `zfs` alone: VMs get virtio hardware,
so the sensor collectors return empty rather than wrong, but the zpools live on
the hypervisor.

Verify with data before adding a flag: `node_scrape_collector_success == 0`
identifies the failing ones, and comparing a guest's series against its Proxmox
host proves duplication.

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

### openipmi

| Variable | Default | Purpose |
|----------|---------|---------|
| `node_exporter_mask_openipmi` | `true` | Mask `openipmi.service` |

Installing `prometheus-node-exporter` drags in an IPMI stack nobody asked for:

```
prometheus-node-exporter
  Recommends: prometheus-node-exporter-collectors
    Recommends: ipmitool
      openipmi
```

No host in this fleet has a BMC — DMI reports no IPMI device and `/dev/ipmi*`
does not exist — so the LSB init script fails to load the IPMI drivers and the
unit stays `failed` forever. It is masked rather than removed for two reasons:
`prometheus-node-exporter-collectors` also produces the `apt`, `nvme` and
`smartmon` textfile metrics, which *are* scraped; and its Recommends would pull
`ipmitool` and `openipmi` straight back in on the next dist-upgrade, whereas a
mask survives a reinstall.

Masking alone is not enough on a host where the unit has already failed:
systemd keeps the recorded failure, so the unit stays in `ActiveState=failed`
and still shows up in `systemctl list-units --state=failed`. The role runs
`reset-failed` after masking to clear it.

Set to `false` on a host that actually has a BMC.

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
