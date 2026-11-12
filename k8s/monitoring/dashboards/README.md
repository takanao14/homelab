# Grafana Dashboards

Define Grafana dashboards in Go with
[grafana-foundation-sdk](https://github.com/grafana/grafana-foundation-sdk) and
generate JSON from those definitions to keep source and deployment artifacts
in sync.

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

- `cmd/poc-v2/`: `dashboardv2` translations of `cmd/generate/` builders
  (ADR-0087), with their registry, helpers, and `layout.go`
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

### Dashboard v2 translations

Translate a builder from `cmd/generate/` into `cmd/poc-v2/`, keeping its
queries, visualization settings, constants, and comments, and add it to
`translations` in `cmd/poc-v2/main.go`. Lay it out for v2 rather than copying
Classic positions: `newLayout` declares rows, each placed by an AutoGrid
(`statGrid`, `chartGrid`, or `wideGrid` in `helpers.go`), and names elements
after panel titles. Pass a fixed, positive panel ID to `Panel(id, builder)`,
using explicitly numbered constants; keep IDs across title/layout changes and
do not reuse retired IDs. Split a row when its panels need different sizes. Wrap
panel queries in `queries(...)`; preserve named references used by overrides
with `queryWithRef(name, query)`.

Until a translation is cut over, `make generate` writes it only to
`poc-generated/` with the UID `<uid>-v2`; the Classic dashboard stays
authoritative. `go test ./...` checks structural invariants; `make check`
checks that committed JSON matches generator output. Compare Classic and v2
with live data: values, series, variable selection, colors, and layout at
desktop and narrow widths:

```bash
make poc-v2-up
```

This starts Grafana 13.2.0 at <http://localhost:3300> with the `.env`
datasources, the chart dashboards in the `chart` folder, and translations in
the `v2` folder. `make poc-v2-stop` removes the container.

Cut over in a separate commit: set `cutover: true`, then delete the Classic
builder and its registry entry in `cmd/generate/`. The translation then
replaces the chart JSON with the same file name and UID. Grafana logs the upstream
[managedFields schema error](https://github.com/grafana/grafana-app-sdk/issues/1485)
once per v2 write without affecting registration or rendering.

## Production

`make generate` writes JSON to `charts/dashboards/dashboards/`.
`make check` regenerates into a temporary directory and detects drift.

```bash
make generate
make check
```
