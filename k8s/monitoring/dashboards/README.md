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
| `dns-overview` | dnsdist + Knot Resolver + pdns-auth + CoreDNS + external-dns: QPS, cache hit rate, latency, response codes, resolver validation logs, record sync |
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
│   ├── validate.go             # Generated-dashboard invariant checks
│   ├── validate_test.go        # Invariant check tests
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
├── setup-linux.sh              # Ubuntu / Rocky toolchain setup (Go, podman-compose)
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
  `measurementThresholds()`, `promDatasource()` / `lokiDatasource()`,
  `promDatasourceVariable()` / `lokiDatasourceVariable()`.

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

Use `issueThresholds()` when the value itself is a verdict: zero is healthy and any
nonzero count is an issue. Use `measurementThresholds()` for quantities whose absolute
value has no universal good/bad meaning, such as throughput, inventory, and total
counts. The latter deliberately supplies one blue step; omitting `Thresholds` is not
equivalent, because a coloured Grafana panel then inherits Grafana's green-below-80 /
red-at-80 default. The generator rejects every non-`none` `ColorMode` that has no
explicit threshold steps.

### Environments and the cluster variable

`prd` and `sandbox` each run their own Prometheus and Grafana, and the
`dashboards` chart is deployed to both with no per-environment values (see
`monitoring.dashboards` in [values/apps-sandbox.yaml](../values/apps-sandbox.yaml)).
Both environments therefore render byte-identical JSON against separate
datasources, and **a dashboard never shows two clusters at once** — in the prd
Grafana the `cluster` variable resolves to exactly one value.

This is why the `cluster` variable, `by (cluster)` groupings, and `{{cluster}}`
legend prefixes are not a way to compare environments. They exist so that one
definition works unchanged in both, and so a legend still says which
environment the browser tab is showing. Keep them: dropping them makes the two
deployments' panels indistinguishable side by side, and breaks the day a
cluster does scrape a second one.

The corollary is that missing data for the *other* environment is never a gap
to fix here. Do not write Go comments or panel descriptions implying a
dashboard spans prd and sandbox — describe what the panel shows, and leave the
environment to the datasource.

### Query idioms

These three are here because getting each of them wrong produced a panel that
looked right. None of them is caught by `make check`, which only compares
generated JSON against the Go source.

**Key a zero baseline on the unfiltered selector.** LogQL emits no sample at all
for an empty window, so a series with nothing to report disappears from a stack
instead of reading zero. The fix is a second term pinned to `$__range` and
multiplied by zero:

```go
Expr(`sum by (host) (count_over_time(` + errSel + `[$__auto]))` +
    ` or sum by (host) (count_over_time(` + base + `[$__range])) * 0`)
```

The baseline must be built from the **selector before the condition being
counted** (`base`), never from the filtered one (`errSel`). Keying it on the
filtered selector appears to work, because it does whenever anything matched,
and collapses to "No data" in exactly the case the baseline exists for — both
terms are empty together. That was a real regression in `syslog`'s error
breakdown. Where a stat reduces to a single value there is no label to preserve
and `or vector(0)` is the correct form.

**Size log windows with `$__auto`, not a literal.** A hardcoded `[5m]` reads
five minutes out of every step once the range is widened, so the panel
under-reports the more you zoom out. `$__auto` tracks the step; `Interval("1m")`
floors it, and because a Min interval raises the step itself the buckets tile
exactly rather than overlapping and counting an event twice. The instant stat
tiles are the exception — their `[1h]` is the quantity the tile reports, not an
artifact of the zoom.

**Plot discrete events as counts, not rates.** A per-second rate suits a volume
panel; it does not suit events arriving ten times a day. One error in a
one-minute bucket is 0.0167, which Grafana renders as `16.7 mc/s`, and the same
single error reads 8.3 mc/s over a day and 1.4 mc/s over a week because the
bucket follows the zoom. `count_over_time` drawn as bars is the same number at
every zoom level for the bucket it sits in, and needs no `round()`: unlike
`increase()` it does not extrapolate.

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

### Panels that hold more series than the panel

A panel whose series count exceeds the space it was given is still readable, but only
if the ordering is chosen rather than incidental. Two cases recur:

- **Bar gauges that scroll.** Wrap the expression in `sort_desc()` so the bars visible
  without dragging are the ones worth seeing; unsorted, they arrive in label order,
  which is whichever hostnames happen to sort first. `node-overview` draws nineteen
  nodes and twenty-seven filesystems into ten grid rows. Sort the whole expression, not
  an inner part of it — order does survive a `group_left` join in practice, but nothing
  documents that it must.
- **Multi-series tooltips.** `defaultTooltip()` lists every series, which runs off the
  bottom of the window past roughly thirty; the series under the cursor can then be the
  one you cannot see. Add `Sort(common.SortOrderDescending)` and a `MaxHeight` below the
  Grafana default of 600. Nothing is lost to the cap: hovering and then clicking pins
  the tooltip, and a pinned tooltip scrolls (confirmed on Grafana 13.1.3).
  `HideZeros(true)` helps less than it sounds — measured on `node-overview`, only 5 of
  54 disk series and 16 of 66 network series sit at exactly zero.

Keep these inline. Which panels are dense is a per-dashboard fact, so this is a pattern
to copy, not a helper to extract (see the litmus test under Conventions).

### Review checklist

- Can an operator identify scope and health without scrolling?
- Does every row name distinguish its contents from adjacent rows?
- Is `Summary`, `Status`, or a component-qualified name chosen by the rules above?
- Do grid spans total 24 on each intended line, with related panels kept together?
- Does the section order support the path from detection to diagnosis to detail?
- Is every zero baseline keyed on the unfiltered selector, so a quiet series still
  reads zero when the filtered one matches nothing?
- Do log windows use `$__auto` except where a fixed window is the reported quantity,
  and do discrete events use `count_over_time` rather than `rate()`?
- Does every panel with more series than it can display order them by value?
- Does every panel with `ColorMode` explicitly define thresholds? An omission applies
  Grafana's unrelated default of green below 80 and red at or above 80.
- Do comments and descriptions avoid implying a dashboard spans environments?
- Are generated JSON files committed together with their Go definitions?

## Development

### Setup

Copy `.env.example` to `.env` and set your Prometheus and Loki URLs:

```bash
cp .env.example .env
# Edit .env
```

Both are required: the Loki datasource backs the log dashboards (`dns-logs`,
`syslog`, `proxmox-logs`, `service-logs`), which render empty without it.

On Linux, `./setup-linux.sh` installs the rest of the toolchain. It supports
Ubuntu 24.04 and Rocky 9/10, and installs:

- Podman and pipx from the distro (Rocky takes pipx from EPEL, which the script
  enables).
- podman-compose through pipx. The project recommends `pip3 install`, but
  Ubuntu 24.04 marks the system Python as externally managed (PEP 668) and
  refuses it, so pipx keeps both distros on one code path.
- Go from the official go.dev tarball into `/usr/local/go`. Neither distro is a
  usable source: Ubuntu's `golang-go` is older than this module's `go` directive
  and is pinned to `GOTOOLCHAIN=local`, so it cannot fetch the required
  toolchain either, and Rocky has no current Go in its base repositories.

The script is idempotent and reads the minimum Go version from `go.mod`, so
re-run it after a `go.mod` bump. It also checks the rootless Podman
prerequisites described below and warns rather than changing anything.

### Start local Grafana

```bash
make dev
```

Opens at http://localhost:3000. Dashboards and the Prometheus/Loki datasources are provisioned automatically.

`make dev` picks the compose implementation itself: real `docker compose` when
available, otherwise `podman-compose`. Force one with
`make dev COMPOSE=podman-compose`. The probe only runs for `dev` / `dev-stop`,
so `generate` and `check` are unaffected.

Two things are ignored on purpose, because both look usable and then fail. The
`podman-docker` package installs a `/usr/bin/docker` shim that forwards to
podman, so the presence of `docker` proves nothing; and podman delegates
`podman compose` to whichever external provider it finds, which on Ubuntu is
often the obsolete `docker-compose` v1 that talks to a Docker socket no Podman
host has. If you do want that path, set it explicitly:
`make dev COMPOSE="podman compose"`.

Under rootless Podman, Grafana runs as UID 472, which maps to a subuid that does
not own the bind-mounted `provisioning/` and dashboard directories. If `$HOME` is
not traversable by others, Grafana starts with no datasources and no dashboards
and logs nothing about it — `chmod o+x "$HOME"`, or add `user: "0"` to the
`grafana` service. The user also needs a `/etc/subuid` range.

On Rocky, SELinux is enforcing by default and blocks a container from reading an
unlabelled bind mount. The volumes in `docker-compose.yml` therefore carry `:z`;
Docker and non-SELinux hosts parse and ignore it.

The image is registry-qualified (`mirror.gcr.io`, the same Docker Hub mirror
[k0s/hook/mirror.sh](../../k0s/hook/mirror.sh) configures for the clusters).
Podman has no implicit default registry, and Ubuntu ships `registries.conf`
without `unqualified-search-registries`, so a short name like
`grafana/grafana:13.0` does not resolve at all.

`setup-linux.sh` checks all of these and warns.

Edit `.go` files in `cmd/generate/`, then re-run `make dev` to reload. Grafana's
file provider rescans `/var/lib/grafana/dashboards` every 10s, so regenerated
JSON is picked up without restarting the container. Datasources are provisioned
only at container start — after editing `.env`, `docker compose up -d` recreates
the container, which reapplies them.

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
