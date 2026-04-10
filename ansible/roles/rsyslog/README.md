# rsyslog Role

Installs and configures rsyslog to forward logs to a remote syslog aggregator (Vector).

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

## Dependencies

None.

## Usage

```yaml
- name: Configure rpi3
  hosts: rpi3
  roles:
    - role: rsyslog
```
