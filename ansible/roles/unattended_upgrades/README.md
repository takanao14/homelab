# unattended_upgrades Role

Installs and enables automatic security upgrades on Debian/Ubuntu hosts via `unattended-upgrades`.

## Functionality

- Installs the `unattended-upgrades` package from APT.
- Deploys `/etc/apt/apt.conf.d/50unattended-upgrades` from a Jinja2 template.
- Writes `/etc/apt/apt.conf.d/20auto-upgrades` to enable daily package list updates and unattended upgrades.

## Upgrade Policy

The deployed config (`50unattended-upgrades`) restricts automatic upgrades to security-related origins only:

- `${distro_id}:${distro_codename}-security`
- `${distro_id}ESMApps:${distro_codename}-apps-security`
- `${distro_id}ESM:${distro_codename}-infra-security`

Automatic reboots are **disabled** (`Automatic-Reboot "false"`). Unused dependencies are removed automatically.

## Variables

None. The upgrade policy is fully defined in the template.

## Dependencies

None.

## Usage

```yaml
- name: Enable unattended security upgrades
  hosts: all:!proxmox
  roles:
    - unattended_upgrades
```
