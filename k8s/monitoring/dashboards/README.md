# Grafana Dashboards

Grafana dashboards are defined in Go with
[grafana-foundation-sdk](https://github.com/grafana/grafana-foundation-sdk) and
generated as JSON.

## Dashboards

| Name | Scope |
|---|---|
| `node-overview` | Bare-metal CPU, memory, temperature, disk, network, and ZFS |
| `k8s-node-overview` | Kubernetes node resources |
| `kubernetes-overview` | Workloads, pods, network, and PVCs |
| `k8s-control-plane` | API server, etcd/kine, CoreDNS, scheduling, and jobs |
| `proxmox-otlp-overview` | Proxmox nodes, guests, storage, network, and PSI |
| `gpu-overview` | AMD GPU activity, VRAM, temperature, power, and clocks |
| `disk-health` | SMART health, failure precursors, wear, and temperature |
| `dns-overview` | dnsdist, resolvers, authoritative DNS, CoreDNS, and external-dns |
| `dns-logs` | DNS query logs |
| `dhcp-leases` | IX2106 DHCP lease inventory, collector health, and lease events |
| `network-overview` | SNMP traffic, errors, discards, and interface state |
| `monitoring-overview` | Prometheus, Alertmanager, and Loki |
| `syslog` | Network-device syslog |
| `proxmox-logs` | Proxmox host journals |
| `service-logs` | Generic journald service logs |
| `cert-manager-overview` | Certificates, issuers, expiry, and sync errors |
| `cilium-overview` | Cilium health, drops, policy, BPF maps, and Hubble |
| `envoy-gateway-overview` | Envoy traffic, latency, and xDS health |
| `argocd-overview` | Argo CD applications, syncs, reconciliation, and Git |
| `openbao-overview` | OpenBao status, requests, Raft, leases, and tokens |
| `slack-bot-overview` | Slack connection, LED delivery, and bot process health |
| `uptime` | ICMP and DNS probe availability |

## Structure

- `cmd/poc-v2/`: isolated Dashboard v2 experiments
- `cmd/generate/main.go`: dashboard registry and JSON output
- `cmd/generate/helpers.go`: shared visual conventions
- `cmd/generate/proxmox_nodes.go`: shared Proxmox inventory loader
- `cmd/generate/node_exporter_targets.go`: node-exporter target classification
- `cmd/generate/validate*.go`: generated-dashboard invariants
- `cmd/generate/*.go`: dashboard definitions
- `provisioning/`: local Grafana datasources and file provider
- `docker-compose.yml`: local Grafana
- `setup-linux.sh`: Ubuntu/Rocky development setup

Add a dashboard builder under `cmd/generate/` and register it in `main.go`.
The Helm chart automatically loads generated JSON from
`charts/dashboards/dashboards/`.

## Conventions

SDK builders are mutable and pointer-backed. Reuse only:

- L0: string constants local to a dashboard builder.
- L1: fragment factories in `helpers.go` that return a new builder on every call.

Do not add whole-panel factories. Per-panel differences make their signatures
opaque and hide the declarative panel list. Extract a helper only when every
call site should change together, and name it by intent
(`issueThresholds`, not `greenRedThresholds`).

Use `issueThresholds()` when zero is healthy and nonzero is a fault. Use
`measurementThresholds()` for neutral quantities such as throughput,
inventory, and totals. A coloured panel must always set thresholds; otherwise
Grafana silently applies its green-below-80/red-at-80 default. The generator
validates this invariant.

### Environments

prd and sandbox deploy byte-identical dashboards against separate datasources.
A dashboard normally sees one cluster, but keeps the `cluster` variable,
grouping, and legend prefix so the shared definition remains environment-aware.
Do not describe missing data from the other environment as a dashboard gap.

### Query idioms

- Key LogQL zero baselines on the unfiltered selector. A filtered selector
  disappears in a quiet window and cannot provide its own zero series.

  ```go
  Expr(`sum by (host) (count_over_time(` + errSel + `[$__auto]))` +
      ` or sum by (host) (count_over_time(` + base + `[$__range])) * 0`)
  ```

  For a single-value stat, use `or vector(0)`.

- Use `$__auto` for zoom-dependent log windows and set `Interval("1m")` to
  floor the step. Fixed windows such as `[1h]` are appropriate when the panel
  explicitly reports that period.

- Plot sparse discrete events with `count_over_time` bars, not per-second
  rates. Use rates for volume.

- A job scraped slower than 30 seconds needs a panel Min interval large enough
  for `$__rate_interval` to contain two samples. SNMP, scraped every minute,
  uses:

  ```go
  const snmpMinInterval = "2m"
  ```

- Aggregate restart-specific `instance` and `pod` labels away, including those
  carried through joins, to avoid splitting historical series. Keep labels
  needed by legends. Use `sum by` for rates and counters and `max by` for
  single-replica gauges to avoid double-counting overlapping pods. Address-based
  external, SNMP, and blackbox targets have stable `instance` labels and need
  no identity aggregation.

- Every label a `LegendFormat` interpolates must survive the query's `by`
  clause. A `{{cluster}}` prefix on top of `sum by (le, route)` renders as a
  leading space.

### Dashboard flow and layout

Order dashboards from detection to diagnosis:

1. Variables narrow scope.
2. Summary/status panels identify immediate issues.
3. Diagnostic rows group related trends.
4. Tables, breakdowns, and logs provide detail.

Use rows for dashboards with multiple concepts and place every panel under one.
Name rows by subject or question, not chart type. Use `Summary` only for a
mixed high-level row; prefer `Status` or a component-qualified name when more
specific.

Grafana uses a 24-column grid. Keep each visual line at 24 columns, place peers
together, and use equal widths unless hierarchy or density requires otherwise.
Prefer spans 4, 6, 8, 12, and 24.

For panels denser than their allocated space:

- Wrap bar-gauge expressions in `sort_desc()`.
- Sort multi-series tooltips and cap their height; pinned tooltips remain
  scrollable.

Keep these choices inline because density is panel-specific.

### Review checklist

- Review rendered dashboards against the query and layout conventions above.
- Confirm scope and health are visible without scrolling and descriptions work
  for either environment.
- Run `make check` and commit generated JSON with its Go definitions.

## Development

Copy `.env.example` to `.env` and set both Prometheus and Loki URLs:

```bash
cp .env.example .env
```

On Ubuntu 24.04 or Rocky 9/10, `./setup-linux.sh` installs Go, Podman, and
podman-compose. It also checks rootless Podman prerequisites and warns without
changing them. The compose mounts use `:z` for SELinux, and the Grafana image
is registry-qualified for hosts without an unqualified registry.

Start Grafana:

```bash
make dev
```

Grafana opens at <http://localhost:3000>. `make dev` chooses Docker Compose or
podman-compose; override it with `COMPOSE=...`. The file provider rescans
generated dashboards every 10 seconds. Restart the container after datasource
or `.env` changes.

Rootless Podman must let container UID 472 traverse and read the bind mounts,
and the user needs a `/etc/subuid` range. Run `setup-linux.sh` for checks.

Stop Grafana with:

```bash
make dev-stop
```

### Dashboard v2 PoC

`make poc-v2-up` starts a separate Grafana 13.2.0 at
<http://localhost:3300/d/uptime-v2-poc> using the same `.env` datasources.
It provisions the uptime, DHCP leases, and cert-manager v2 dashboards.
The Classic comparison is at <http://localhost:3300/d/uptime>.
`make poc-v2-stop` removes the PoC container.

`make poc-v2` generates local copies in `poc-generated/`.
`make generate` also includes it in the shared dashboard Helm chart, so prd
and sandbox receive the v2 PoCs alongside their Classic dashboards through the
existing ConfigMap sidecar. Their separate UIDs are `uptime-v2-poc`,
`dhcp-leases-v2-poc`, and `cert-manager-v2-poc`.
`go test ./...` checks v2 resources against committed Classic JSON and validates
references, IDs, and thresholds. Compare values, colors, legends, row collapse,
datasource switching, and regeneration using the same datasource and absolute
time range.

All three v2 dashboards were visually verified on prd Grafana 13.2.2; sandbox
sidecar compatibility remains unverified. The upstream
[managedFields schema error](https://github.com/grafana/grafana/issues/128991)
was logged without preventing registration or rendering. Classic dashboards
remain authoritative under ADR-0007.

## Production

`make generate` writes JSON to `charts/dashboards/dashboards/`.
`make check` regenerates into a temporary directory and detects drift.

```bash
make generate
make check
```
