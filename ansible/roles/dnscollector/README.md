# dnscollector Role

DNS-collector collects dnsdist query and response events for DNS traffic
investigation in Loki. The role installs [DNS-collector](https://github.com/dmachard/DNS-collector)
to receive dnstap events over a Unix socket and forward them to Loki; debug mode
sends them to stdout instead.

## Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `dnscollector_version` | [defaults/main.yaml](defaults/main.yaml) | Binary version to install |
| `dnscollector_loki_url` | `{{ loki_endpoint }}` | Loki push endpoint |
| `dnscollector_dnsdist_socket` | `/run/dnstap-dnsdist.sock` | dnstap Unix socket path (shared with dnsdist) |
| `dnscollector_debug` | `false` | Enable debug logging |

Architecture is auto-detected from `ansible_facts['architecture']` (supports `x86_64` and `aarch64`).

## Dependencies

- `dnsdist` role (provides the dnstap socket)

## Usage

```yaml
- name: Setup dnsdist
  hosts: dnsdist
  roles:
    - role: dnscollector
    - role: dnsdist
```
