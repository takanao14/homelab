# dnscollector Role

Installs and configures [DNS-collector](https://github.com/dmachard/DNS-collector) as a DNS traffic logger. Receives dnstap events from dnsdist via a Unix socket and forwards them to Loki.

## Functionality

- Downloads the dnscollector binary from GitHub releases.
- Deploys `/etc/dnscollector/config.yaml` from a Jinja2 template.
- Deploys and enables a systemd unit.
- Ensures the service is started and enabled.

## Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `dnscollector_version` | `2.2.1` | Binary version to install |
| `dnscollector_loki_url` | `http://192.168.30.134:3100` | Loki push endpoint |
| `dnscollector_dnsdist_socket` | `/run/dnstap-dnsdist.sock` | dnstap Unix socket path (shared with dnsdist) |
| `dnscollector_debug` | `false` | Enable debug logging |

Architecture is auto-detected from `ansible_facts['architecture']` (supports `x86_64` and `aarch64`).

## Dependencies

- `dnsdist` role (provides the dnstap socket)

## Usage

```yaml
- name: Setup DNS Servers
  hosts: dns_primary
  roles:
    - role: dnscollector
    - role: dnsdist
```
