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
| `uptime` | ICMP and DNS probe availability |

## Structure

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

- Can scope and health be identified without scrolling?
- Do row names and section order support detection, diagnosis, and detail?
- Do intended grid lines total 24 columns?
- Are LogQL zero baselines keyed on unfiltered selectors?
- Do zoom-dependent log queries use `$__auto`?
- Are sparse events counts rather than rates?
- Do slow-scrape rate panels set a Min interval?
- Are dense panels ordered by value?
- Does every coloured panel define thresholds?
- Are descriptions environment-neutral?
- Are generated JSON files updated with their Go definitions?

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

## Production

`make generate` writes JSON to `charts/dashboards/dashboards/`.
`make check` regenerates into a temporary directory and detects drift.

```text
Edit Go → make generate → make check → commit Go and JSON → Argo CD sync
```

```bash
make generate
make check
```
