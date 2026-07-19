# Grafana Dashboards

Grafana dashboards are defined as Go code using [grafana-foundation-sdk](https://github.com/grafana/grafana-foundation-sdk) and generated as JSON.

## Dashboards

| Name | Description |
|---|---|
| `node-overview` | Bare-metal nodes: CPU, memory, temperature, disk I/O, network I/O, ZFS ARC |
| `k8s-node-overview` | Kubernetes nodes: CPU, memory, disk, network (filtered by cluster/node) |
| `kubernetes-overview` | Kubernetes cluster health, resource usage, pod lifecycle, network, PVC |
| `k8s-control-plane` | Kubernetes control plane and DNS: API server, etcd/kine backend, CoreDNS, scheduling, capacity, jobs |
| `proxmox-otlp-overview` | Proxmox VE cluster (native OTLP metrics): VM/LXC counts, node and guest resources, storage, network I/O, PSI pressure |
| `gpu-overview` | AMD RX 9060 XT: activity, VRAM, temperature, power, clock speed |
| `disk-health` | Physical disk S.M.A.R.T.: health flag, failure precursors, SSD wear, temperature |
| `dns-overview` | dnsdist + pdns-auth + CoreDNS + external-dns: QPS, cache hit rate, latency, response codes, record sync |
| `dns-logs` | DNS query logs via Loki: query rate, response codes, top domains, per-host breakdown |
| `network-overview` | SNMP MIB-II (bgw1/c1200): traffic, errors, discards, interface status |
| `monitoring-overview` | Prometheus, Alertmanager, and Loki self-monitoring: alerts, scrape targets, TSDB, ingestion rate |
| `syslog` | Network device syslog volume and error rate via Loki |
| `proxmox-logs` | Proxmox VE host journals via Loki: node/service errors, warnings, and operational signals |
| `service-logs` | Generic journald service logs via Loki: volume, errors/warnings by unit |
| `cert-manager-overview` | cert-manager certificates and ClusterIssuers: expiry countdown, ready state, sync errors |
| `cilium-overview` | Cilium CNI: agent/operator health, packet drops, policy verdicts, BPF map pressure, endpoint state, Hubble flows |
| `envoy-gateway-overview` | Envoy Gateway: listener/HTTPRoute traffic, response codes, upstream latency, xDS sync health |
| `argocd-overview` | ArgoCD: app health/sync status, sync failures, reconcile latency, repo-server git requests |
| `openbao-overview` | OpenBao (external VM): seal/active status, request rate and latency, raft storage, leases and tokens |
| `uptime` | ICMP/DNS probe availability timeline |

## Structure

```
.
├── cmd/generate/               # Dashboard definitions (Go)
│   ├── main.go                 # Entrypoint (dashboard registry + JSON output)
│   ├── helpers.go              # Shared house-style helpers (see Conventions)
│   ├── node.go                 # node-overview
│   ├── k8s_node.go             # k8s-node-overview
│   ├── kubernetes.go           # kubernetes-overview
│   ├── k8s_control_plane.go    # k8s-control-plane
│   ├── proxmox_otlp.go         # proxmox-otlp-overview
│   ├── gpu.go                  # gpu-overview
│   ├── disk_health.go          # disk-health
│   ├── dns.go                  # dns-overview
│   ├── dns_logs.go             # dns-logs
│   ├── network.go              # network-overview
│   ├── monitoring.go           # monitoring-overview
│   ├── syslog.go               # syslog
│   ├── proxmox_logs.go         # proxmox-logs
│   ├── service_logs.go         # service-logs
│   ├── cert_manager.go         # cert-manager-overview
│   ├── cilium.go               # cilium-overview
│   ├── envoy_gateway.go        # envoy-gateway-overview
│   ├── argocd.go               # argocd-overview
│   ├── openbao.go              # openbao-overview
│   └── uptime.go               # uptime
├── provisioning/               # Local Grafana provisioning config
│   ├── datasources/            # Prometheus datasource
│   └── dashboards/             # Dashboard file provider
├── docker-compose.yml          # Local development Grafana
└── Makefile
```

To add a new dashboard, create a new `.go` file in `cmd/generate/` (e.g., `new_dashboard.go`) and add an entry to the `dashboards` map in `cmd/generate/main.go`.
The Helm template auto-discovers all JSON files in `charts/dashboards/dashboards/`, so no template changes are needed.

## Conventions

The foundation SDK is a DSL whose builders are **mutable, pointer-backed objects**.
Shared helpers therefore keep reuse to two levels and no further:

- **L0 — string constants.** PromQL fragments and label filters (e.g. `fsFilter`,
  `joinNodename`) are declared as `const` within each builder.
- **L1 — fragment factories.** Repeated style/config defaults live as functions in
  `helpers.go` that **return a fresh builder on every call**: `defaultTooltip()`,
  `defaultLegend()`, `zeroLineThresholds()`, `zeroLineStyle()`, `issueThresholds()`,
  `promDatasource()` / `lokiDatasource()`, `promDatasourceVariable()` /
  `lokiDatasourceVariable()`.

We deliberately **do not introduce L2 panel factories** (helpers that assemble whole
panels from parameters). They accrete arguments to absorb per-panel differences and
obscure the SDK's main strength: each `build*` function reads as a declarative list of
what panels a dashboard contains.

Guidelines when adding helpers:

- A helper must return a **new builder instance** each call — never a shared package
  variable — to avoid aliasing bugs between panels.
- Extract only **conventions** (decisions that should change everywhere at once, e.g.
  legend placement, the green→red "any nonzero is an issue" threshold). Leave
  incidental look-alike code inline.
- Litmus test: *"If I change this helper, do I want every call site to change?"*
  Yes → helper. "Depends on the panel" → keep it inline.
- Name helpers by **intent, not shape** (`issueThresholds`, not `greenRedThresholds`).

## Dashboard structure guidelines

Dashboards should tell the same operational story from top to bottom: establish scope,
show whether immediate action is required, provide diagnostic trends, and finish with
high-cardinality detail. Organize panels in this order unless the dashboard has a
clearer domain-specific investigation path:

1. **Variables** narrow the dashboard scope (environment, cluster, node, service, or
   datasource). Put broad selectors before dependent or narrower selectors.
2. **Overview** shows the few current values needed to decide whether to investigate.
   Prefer status and issue counters over capacity or activity metrics.
3. **Diagnostics** groups related trends by operational domain, such as CPU, traffic,
   storage, latency, or control-plane component.
4. **Detail** provides tables, breakdowns, and logs used after a problem is identified.

### Rows and section names

- Use rows whenever a dashboard contains more than one conceptual section. Once rows
  are used, place every panel under a row; do not leave an unnamed panel group at the
  top or between named sections.
- A small, single-purpose dashboard may omit rows only when all panels form one
  uninterrupted investigation flow. Add rows as soon as a second conceptual section
  appears.
- Name a row for the **question or subject shared by its panels**, not for a chart type
  (`Metrics`, `Charts`) or an implementation detail.
- Use `Summary` for the first row of a single-subject dashboard when it mixes several
  kinds of high-level signals. Examples: node health, utilization, and issue counts.
- Use a more specific first-row name such as `Status` or `Cluster Health` when every
  panel answers that narrower question. Do not add `Summary` mechanically.
- For a dashboard covering multiple peer components, prefix every component section
  consistently. Use `<Component> Summary` for its overview and descriptive names for
  subsequent sections, such as `<Component> Traffic` or `<Component> Storage`.
- Avoid a bare component name when the dashboard also contains additional sections for
  that component; `Prometheus Summary` is clearer beside `Prometheus Metrics` than
  `Prometheus` is.
- Use concise English Title Case, preserve official product capitalization, and prefer
  nouns or noun phrases. Use `&` for paired concepts (`Errors & Warnings`).

### Panel layout

- Grafana rows use a 24-column grid. Each visual line should total 24 columns; avoid
  accidental trailing whitespace or a single narrow panel wrapping onto its own line.
- Arrange panels by operational priority from left to right and top to bottom. Keep
  directly comparable panels adjacent (for example, targets up/down or requests/errors).
- Use equal widths for peers. Use unequal widths only to express a real hierarchy or to
  give a dense visualization more reading space.
- Prefer the standard spans `4`, `6`, `8`, `12`, and `24`. Summary stat panels normally
  use height `3` or `4`; trend panels normally use height `8` unless their content needs
  more space.
- Keep one semantic group on one visual line when practical. If a group must wrap,
  split it into balanced lines based on meaning rather than insertion order.
- Do not shrink panels merely to force a one-line layout. Titles, legends, and values
  must remain readable at the dashboard's expected viewport width.

### Review checklist

- Can an operator identify scope and health without scrolling?
- Does every row name distinguish its contents from adjacent rows?
- Is `Summary`, `Status`, or a component-qualified name chosen by the rules above?
- Do grid spans total 24 on each intended line, with related panels kept together?
- Does the section order support the path from detection to diagnosis to detail?
- Are generated JSON files committed together with their Go definitions?

## Development

### Setup

Copy `.env.example` to `.env` and set your Prometheus URL:

```bash
cp .env.example .env
# Edit .env
```

### Start local Grafana

```bash
make dev
```

Opens at http://localhost:3000. Dashboards and the Prometheus datasource are provisioned automatically.

Edit `.go` files in `cmd/generate/`, then re-run `make dev` to reload.

### Stop

```bash
make dev-stop
```

## Production

`make generate` writes JSON directly to `charts/dashboards/dashboards/`. Commit the generated JSON with the Go source changes so ArgoCD can sync and roll back entirely from Git. CI runs `make check`, which regenerates dashboards into a temporary directory and fails if the committed chart JSON has drifted.

```
Edit .go files in cmd/generate/
  → make generate
  → make check
  → git commit & push
  → CI verifies generated JSON drift
  → ArgoCD syncs ConfigMaps
  → Grafana sidecar reloads dashboards
```

```bash
make generate
make check
git add ../charts/dashboards/dashboards/
git commit -m "..."
```
