# journald Role

Manages whether systemd-journald forwards journal entries to the traditional
syslog socket.

## Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `journald_forward_to_syslog` | `false` | Set `ForwardToSyslog` in a journald drop-in |
| `journald_forward_to_syslog_dropin` | `zz-forward-to-syslog.conf` | Late-sorting drop-in filename that overrides vendor files such as Ubuntu's `syslog.conf` |
| `journald_disable_rsyslog` | `false` | Stop/disable rsyslog and mask `syslog.socket` when another agent reads the journal directly |

The role restarts `systemd-journald` when the drop-in changes. Journal clients
continue to use the systemd-managed sockets during the restart.
