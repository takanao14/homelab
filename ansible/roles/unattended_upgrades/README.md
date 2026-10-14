# unattended_upgrades Role

Installs and enables automatic security upgrades on Debian/Ubuntu hosts via `unattended-upgrades`.

## Upgrade Policy

The deployed config (`50unattended-upgrades`) restricts automatic upgrades to security-related origins only:

- `${distro_id}:${distro_codename}-security`
- `${distro_id}ESMApps:${distro_codename}-apps-security`
- `${distro_id}ESM:${distro_codename}-infra-security`

Automatic reboots are **disabled** (`Automatic-Reboot "false"`). Unused dependencies are removed automatically.

## Variables

None. The upgrade policy is fully defined in the template.

## Usage

Run [playbooks/common/unattended_upgrades.yaml](../../playbooks/common/unattended_upgrades.yaml).
