# chrony Role

Configures the [chrony](https://chrony-project.org/) NTP daemon to synchronize
time against the on-prem router (the single internal NTP server) instead of
public pools. Clients point only at the router; external-source redundancy is
handled on the router (multiple upstreams on the IX), not per-client.

The role disables unmanaged main-config sources and writes managed sources to
`/etc/chrony/sources.d/homelab.sources`. DHCP-provided sources remain enabled;
use `chrony_disabled_source_dirs` to exclude untrusted dynamic sources.

On Debian/Ubuntu, it stops and disables `systemd-timesyncd` without masking it,
unless `chrony_manage_timesyncd` is false. Absent units are tolerated. Config
paths and service names for Debian/Ubuntu and RedHat/Rocky come from
`vars/<os_family>.yaml`.

## Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `chrony_install` | `true` | Install the chrony package. Set `false` to manage config only (e.g. Proxmox hypervisors, see `group_vars/proxmox.yaml`). |
| `chrony_manage_timesyncd` | `true` | Stop/disable `systemd-timesyncd` (Debian/Ubuntu only). Set `false` on hosts already running chrony to leave systemd state untouched (e.g. Proxmox). |
| `chrony_servers` | `[192.168.10.1]` | NTP source(s) — the router (single internal NTP server). |
| `chrony_fallback_servers` | `[]` | Optional extra sources appended after `chrony_servers`. Empty by default: external-source redundancy is handled on the router, not per-client. |
| `chrony_disabled_source_dirs` | `[]` | Optional `sourcedir` paths to comment out when a dynamic source directory is untrusted or noisy. DHCP-provided sources are allowed by default. |
| `chrony_source_dir_path` | `/etc/chrony/sources.d` | Source directory included from the main chrony config. |
| `chrony_managed_sources_path` | `/etc/chrony/sources.d/homelab.sources` | Managed chrony source file containing homelab NTP sources. |

## Scope

Applied to `all:!lxc:!power_only` (see `playbooks/common/chrony.yaml`). LXC containers are excluded:
unprivileged containers lack `CAP_SYS_TIME` and inherit the host clock, so they
must not run their own NTP daemon.

## Usage

Run [playbooks/common/chrony.yaml](../../playbooks/common/chrony.yaml).
