# chrony Role

Configures the [chrony](https://chrony-project.org/) NTP daemon to synchronize
time against the on-prem router (the single internal NTP server) instead of
public pools. Clients point only at the router; external-source redundancy is
handled on the router (multiple upstreams on the IX), not per-client.

## Functionality

- Optionally installs the `chrony` package (`chrony_install`).
- On Debian/Ubuntu, stops and disables `systemd-timesyncd` (the default time
  sync) so it does not compete with chrony. It is intentionally **not masked**
  (installing chrony already deactivates it). Tolerated as a no-op where the unit
  is absent (minimal images); skipped on RedHat/Rocky (no timesyncd) and where
  `chrony_manage_timesyncd` is false.
- Comments out unmanaged active `server`, `pool`, and `peer` directives in the
  chrony config so only the Ansible-managed sources remain active.
- Leaves dynamic source directories (for example `/run/chrony-dhcp`) enabled by
  default. DHCP-provided NTP servers are accepted as site policy when DHCP is
  controlled; `chrony_disabled_source_dirs` is available as an opt-in escape
  hatch for untrusted or noisy dynamic sources.
- Writes the Ansible-managed `server` directives (the router, plus any optional
  `chrony_fallback_servers`) into the chrony source directory
  (`/etc/chrony/sources.d/homelab.sources`).
- Ensures the source directory is included from the main chrony config with
  `sourcedir /etc/chrony/sources.d`.
- Removes the legacy Ansible-managed block from the main chrony config if it
  exists from older role runs.
- Enables and starts the chrony service, restarting it on config change.

The role is OS-aware (Debian/Ubuntu and RedHat/Rocky) via `vars/<os_family>.yaml`
for the config path and service name.

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

Applied to `all:!lxc` (see `playbooks/common-chrony.yaml`). LXC containers are excluded:
unprivileged containers lack `CAP_SYS_TIME` and inherit the host clock, so they
must not run their own NTP daemon.

## Usage

```yaml
- name: Configure chrony
  hosts: all:!lxc
  roles:
    - chrony
```
