# vector Role

Installs and configures [Vector](https://vector.dev/) as a log aggregator on Debian-based systems. Receives syslog (UDP 514) from external hosts and forwards to Loki.

## Functionality

- Adds the Vector APT repository declaratively (`deb822_repository`, signed by
  Datadog's current apt key) and removes the legacy `vector.list` left behind by
  the old `setup.vector.dev` script.
- Installs the `vector` package.
- Deploys `/etc/vector/vector.yaml` from a Jinja2 template.
- Validates the installed configuration on every normal run, including when a
  package upgrade changes the Vector binary without changing the template.
- Keeps check mode read-only when `python3-debian` is not installed yet; the
  repository task is reported as deferred until the prerequisite is applied.
- Defers package-dependent configuration and service checks on a pristine host
  during check mode because a repository pending creation is not yet visible to
  APT.

## Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `vector_repo_url` | `https://apt.vector.dev/` | Vector apt repository URL |
| `vector_repo_key_url` | `https://keys.datadoghq.com/DATADOG_APT_KEY_CURRENT.public` | Repository signing key |
| `vector_loki_endpoint` | `http://loki:3100` | Loki push endpoint |
| `vector_config` | `{}` | Vector pipeline configuration (sources, transforms, sinks) |

`vector_config` is passed directly into the template. Define it in
`group_vars/log_collector.yaml` for the central collector.

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

## Usage

Run [playbooks/common-vector.yaml](../../playbooks/common-vector.yaml).

## Notes

- Vector 0.57 introduced template confinement for sink fields. Loki sinks that
  use event fields as complete label values explicitly set
  `dangerously_allow_unconfined_template_resolution: true` in inventory to
  preserve the existing label values. Keep this exception visible per sink;
  adding a static prefix would change labels and break existing Loki queries.

On `log1`, `log_collector.yaml` parses `led-server` JSON into `app`, `level`, and `event` while retaining the original syslog `message` and `severity`. These fields stay in the payload, not Loki labels. Non-JSON LED logs are retained with `app_parse_error=true`; other applications pass through unchanged. Run `vector test /etc/vector/vector.yaml` to check the embedded cases. Query with `{host="rpi3",appname="led-server"} | json | level="error"` or filter `event`.
