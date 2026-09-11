# vector Role

Installs and configures [Vector](https://vector.dev/) on Debian-based systems. Most hosts ship their own journal to Loki; `log1` additionally receives syslog (UDP/TCP 514) from hosts and network appliances that cannot.

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
- Optionally exposes Vector internal metrics through a Prometheus exporter
  bound to an inventory-selected address.

## Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `vector_repo_url` | `https://apt.vector.dev/` | Vector apt repository URL |
| `vector_repo_key_url` | `https://keys.datadoghq.com/DATADOG_APT_KEY_CURRENT.public` | Repository signing key |
| `vector_journald_units` | `[]` | Units the standard pipeline collects |
| `vector_loki_extra_labels` | `{}` | Labels merged into the standard `host`/`unit` pair |
| `vector_internal_metrics_enabled` | `false` | Add an `internal_metrics` source and Prometheus exporter sink |
| `vector_internal_metrics_address` | `127.0.0.1:9598` | Prometheus exporter listen address when enabled |
| `vector_config` | standard pipeline | Full pipeline (sources, transforms, sinks) |

The default `vector_config` is a journald-to-Loki pipeline labelled by `host`
and `unit`, so a normal host only declares which units to collect:

```yaml
vector_journald_units:
  - caddy
  - ssh
```

`vector_config` is passed directly into the template, so a host that needs a
different pipeline replaces it entirely in inventory. `log_collector` (syslog
reception plus led-server parsing) and `rpi4` (DHCP lease JSON merged into the
event) do this; inventory outranks these defaults, so such a host must carry
`since_now` in its own journald source.

The `vector_vm` inventory group enables internal metrics on each VM's managed
address. The observability API remains disabled; it is unauthenticated and is
not needed for Prometheus collection.

Loki index labels stay low-cardinality. Keep request paths, users, and source
IPs in the payload, where LogQL can still filter them.

## Usage

Run [playbooks/common-vector.yaml](../../playbooks/common-vector.yaml).

## Notes

- Vector 0.57 introduced template confinement for sink fields. Loki sinks that
  use event fields as complete label values explicitly set
  `dangerously_allow_unconfined_template_resolution: true` to preserve the
  existing label values. Keep this exception visible per sink;
  adding a static prefix would change labels and break existing Loki queries.

On `log1`, `log_collector.yaml` parses `led-server` JSON into `app`, `level`, and `event` while retaining the original syslog `message` and `severity`. These fields stay in the payload, not Loki labels. Non-JSON LED logs are retained with `app_parse_error=true`; other applications pass through unchanged. Run `vector test /etc/vector/vector.yaml` to check the embedded cases. Query with `{host="rpi3",appname="led-server"} | json | level="error"` or filter `event`.
