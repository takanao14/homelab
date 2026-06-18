# sysctl Role

Manages persistent kernel parameters via a `sysctl.d` drop-in using the
`ansible.posix.sysctl` module. Each parameter is written to `sysctl_conf_file`
and applied live (`reload: true`), so no reboot is required.

## Functionality

- Renders every key in `sysctl_settings` into `sysctl_conf_file`.
- Verifies the running value matches (`sysctl_set: true`) and reloads.

## Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `sysctl_conf_file` | `/etc/sysctl.d/99-ansible.conf` | Drop-in file the parameters are written to |
| `sysctl_settings` | `{ vm.swappiness: 10 }` | Map of `sysctl` key -> value. Replaces (does not merge) when overridden |

### Default rationale

- `vm.swappiness: 10` — lowered from the Debian/Proxmox default of `60`, which
  swaps out live VM memory pre-emptively and causes guest latency spikes. `10`
  keeps swap as a safety valve under real pressure without swapping eagerly. Do
  not use `0` (risks OOM-killing qemu instead of a mild swap-out).

## Usage

```yaml
- name: Tune kernel parameters
  hosts: proxmox
  roles:
    - role: sysctl
      tags: sysctl
```

Override per group/host (e.g. to also bound the ZFS ARC). Note the whole dict is
replaced, so re-state `vm.swappiness`:

```yaml
# group_vars/proxmox.yaml
sysctl_settings:
  vm.swappiness: 10
  # Cap the ZFS ARC at 8 GiB so it does not contend with running VMs and force
  # swapping. Set this only on ZFS-backed hosts.
  # zfs_arc tuning is a module param, not a sysctl; see /etc/modprobe.d instead.
```

## Dependencies

- `ansible.posix` Ansible collection.
