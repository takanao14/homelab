# unattended_upgrades Role

Installs and enables automatic security upgrades on Debian/Ubuntu hosts via `unattended-upgrades`.

## Upgrade Policy

The `50unattended-upgrades` config allows only security-related origins to
apply security fixes automatically while keeping other package upgrades under
operator control:

- `${distro_id}:${distro_codename}-security`
- `${distro_id}ESMApps:${distro_codename}-apps-security`
- `${distro_id}ESM:${distro_codename}-infra-security`

`Automatic-Reboot "false"` disables automatic reboots to keep service
disruption within operator-controlled maintenance. Unused dependencies are
removed automatically.

## Variables

None. Edit the template to change the shared upgrade policy.

## Usage

Run [playbooks/common/unattended_upgrades.yaml](../../playbooks/common/unattended_upgrades.yaml).
