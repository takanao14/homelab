# rsyslog Role

Installs and configures rsyslog to forward logs to a remote Vector log collector.

## Functionality

- Installs `rsyslog` from APT.
- Deploys `/etc/rsyslog.d/99-forward.conf` from a Jinja2 template.
- Ensures the `rsyslog` service is started and enabled.

## Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `rsyslog_forward_host` | `192.168.10.243` | Destination syslog host |
| `rsyslog_forward_port` | `514` | Destination port |
| `rsyslog_forward_protocol` | `udp` | Protocol (`udp` or `tcp`) |
| `rsyslog_journal_input` | `false` | Read journald directly with `imjournal` and use an isolated forwarding ruleset |
| `rsyslog_ignore_previous_journal` | `true` | Start at the end of the journal on the first deployment |
| `rsyslog_queue_size` | `10000` | Maximum number of messages in the memory queue |
| `rsyslog_queue_high_watermark` | `9000` | Queue depth that starts disk-assisted spooling |
| `rsyslog_queue_low_watermark` | `7000` | Queue depth that stops disk-assisted spooling |
| `rsyslog_queue_max_disk_space` | `256m` | Maximum disk space for the forwarding queue |

When `rsyslog_journal_input` is enabled, the journal input is connected directly
to a dedicated forwarding ruleset. The forwarding action uses a bounded
disk-assisted memory queue and drops new remote-forwarding entries when the
queue is full rather than blocking the source host.

## Dependencies

None.

## Usage

```yaml
- name: Configure rpi3
  hosts: rpi3
  roles:
    - role: rsyslog
```
