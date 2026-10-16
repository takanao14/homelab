# sysctl Role

Manages persistent kernel parameters via a `sysctl.d` drop-in using the
`ansible.posix.sysctl` module. The role writes each parameter to
`sysctl_conf_file` and applies it live (`reload: true`) to avoid a reboot.

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

Overrides replace the whole `sysctl_settings` map, so retain
`vm.swappiness` when adding parameters. ZFS ARC sizing is a module parameter
under `/etc/modprobe.d`, not a sysctl.

## Dependencies

- `ansible.posix` Ansible collection.
