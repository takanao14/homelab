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
| `dns-overview` | dnsdist + pdns-auth: QPS, cache hit rate, latency, response codes, drop rate |
| `dns-logs` | DNS query logs via Loki: query rate, response codes, top domains, per-host breakdown |
| `network-overview` | SNMP MIB-II (bgw1/c1200): traffic, errors, discards, interface status |
| `monitoring-overview` | Prometheus, Alertmanager, and Loki self-monitoring: alerts, scrape targets, TSDB, ingestion rate |
| `syslog` | Network device syslog volume and error rate via Loki |
| `service-logs` | Generic journald service logs via Loki: volume, errors/warnings by unit |
| `cert-manager-overview` | cert-manager certificates and ClusterIssuers: expiry countdown, ready state, sync errors |
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
│   ├── service_logs.go         # service-logs
│   └── uptime.go               # uptime
├── generated/                  # Generated JSON output (git-ignored)
├── provisioning/               # Local Grafana provisioning config
│   ├── datasources/            # Prometheus datasource
│   └── dashboards/             # Dashboard file provider
├── docker-compose.yml          # Local development Grafana
└── Makefile
```

To add a new dashboard, create a new `.go` file in `cmd/generate/` (e.g., `new_dashboard.go`) and add an entry to the `dashboards` map in `cmd/generate/main.go`.
The Helm template auto-discovers all JSON files in `charts/prometheus/dashboards/`, so no template changes are needed.

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

`make deploy` generates JSON and copies it to `charts/prometheus/dashboards/`. Commit the diff and ArgoCD will sync the updated ConfigMaps to the cluster.

```
Edit .go files in cmd/generate/
  → make deploy  (generate JSON + copy to Helm chart)
  → git commit & push
  → ArgoCD syncs ConfigMaps
  → Grafana sidecar reloads dashboards
```

```bash
make deploy
git add ../charts/prometheus/dashboards/
git commit -m "..."
```
