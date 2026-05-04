# blackbox_exporter Role

Installs and configures [Prometheus Blackbox Exporter](https://github.com/prometheus/blackbox_exporter) on Debian-based systems.

## Functionality

- Installs `prometheus-blackbox-exporter` and `libcap2-bin` from APT.
- Deploys the Blackbox Exporter config from a Jinja2 template to `blackbox_exporter_config_path`.
- Grants `CAP_NET_RAW` to the binary via `setcap` (required for ICMP probes).
- Ensures the service is started and enabled.

## Probe Modules

The deployed config defines three modules:

| Module | Prober | Description |
|--------|--------|-------------|
| `icmp` | icmp | ICMP ping (IPv4) |
| `dns_external` | dns | DNS query for `google.com` against the target resolver |
| `dns_internal` | dns | DNS query for `blackbox_exporter_dns_internal_query` against the target resolver |

## Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `blackbox_exporter_port` | `9115` | Port the exporter listens on |
| `blackbox_exporter_config_path` | `/etc/prometheus/blackbox.yml` | Path to the deployed config file |
| `blackbox_exporter_dns_internal_query` | `ns1.home.butaco.net` | Hostname used for the `dns_internal` probe |

## Dependencies

None.

## Usage

```yaml
- name: Install and configure prometheus-blackbox-exporter
  hosts: blackbox_exporter
  roles:
    - blackbox_exporter
```
