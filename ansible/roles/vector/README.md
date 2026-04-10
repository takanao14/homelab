# vector Role

Installs and configures [Vector](https://vector.dev/) as a log aggregator on Debian-based systems. Receives syslog (UDP 514) from external hosts and forwards to Loki.

## Functionality

- Adds the Vector APT repository via the official setup script.
- Installs the `vector` package.
- Deploys `/etc/vector/vector.yaml` from a Jinja2 template.
- Ensures the `vector` service is started and enabled.

## Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `vector_loki_endpoint` | `http://loki:3100` | Loki push endpoint |
| `vector_config` | `{}` | Vector pipeline configuration (sources, transforms, sinks) |

`vector_config` is passed directly into the template. Define it in `group_vars/syslog.yaml`.

Example:

```yaml
vector_config:
  sources:
    syslog_in:
      type: syslog
      address: "0.0.0.0:514"
      mode: udp
  sinks:
    loki_out:
      type: loki
      inputs: [syslog_in]
      endpoint: "{{ vector_loki_endpoint }}"
```

## Dependencies

None.

## Usage

```yaml
- name: Setup Syslog Aggregator
  hosts: syslog
  roles:
    - role: vector
```

## Notes

- Repository setup uses the official `setup.vector.dev` script with `creates:` guard for idempotency.
