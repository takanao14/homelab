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

Only amd64 (`x86_64`) hosts are supported. Renovate updates `dnscollector_version`
and `dnscollector_sha256` together; CI verifies the pinned archive checksum.

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
