# journald Role

Manages systemd-journald's syslog forwarding and journal storage.

## Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `journald_forward_to_syslog` | `false` | Set `ForwardToSyslog` in a journald drop-in |
| `journald_forward_to_syslog_dropin` | `zz-forward-to-syslog.conf` | Late-sorting drop-in filename that overrides vendor files such as Ubuntu's `syslog.conf` |
| `journald_disable_rsyslog` | `false` | Stop/disable rsyslog and mask `syslog.socket` when another agent reads the journal directly |
| `journald_storage` | `auto` | `Storage` value; `auto` removes the drop-in and leaves the distribution default |
| `journald_storage_dropin` | `zz-storage.conf` | Drop-in filename for the storage override |
| `journald_system_max_use` | `""` | Optional `SystemMaxUse` cap, written alongside a non-`auto` storage |

The role restarts `systemd-journald` when a drop-in changes. Journal clients
continue to use the systemd-managed sockets during the restart. The `zz-` prefix
sorts after vendor drop-ins that would otherwise win, such as Raspberry Pi OS's
`40-rpi-volatile-storage.conf`.
