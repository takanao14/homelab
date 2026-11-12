# ADR-0087: Migrate the dashboard generator to the dashboardv2 SDK API

- **Status:** Accepted
- **Category:** Observability
- **Date:** 2026-11-11
- **Last reviewed:** 2026-11-12
- **Review trigger:** The first rewritten dashboard has replaced its Classic
  version, with the same file name and UID, through the sandbox ConfigMap
  sidecar.
- **Related:** [ADR-0007](0007-defer-grafana-dashboard-v2-migration.md),
  [`k8s/monitoring/dashboards`](../../k8s/monitoring/dashboards/README.md)

## Context

The generator in `k8s/monitoring/dashboards/cmd/generate/` builds 22 dashboards
with the Foundation SDK `dashboard` package, which the SDK deprecates in favor
of `dashboardv2`. ADR-0007 deferred the move until Grafana fixed the Dashboard
v2 managed-fields schema error, the SDK supported Prometheus query variables,
and a stable v2 API existed, and required a superseding ADR before migrating.

As of 2026-11-11 only the last trigger is met: the v2 schema became generally
available in Grafana 13.0. SDK `v0.0.22`, the latest release, still has no
Prometheus variable-query builder; a local one is about 15 lines. The
managed-fields error, now tracked as
[grafana-app-sdk#1485](https://github.com/grafana/grafana-app-sdk/issues/1485),
is logged once per v2 write without affecting registration or rendering.

`dashboardv2` is not a package rename. It separates elements from layout:
`WithRow` / `WithPanel` with `Span` and `Height` become a panel map plus a
rows layout of grid items with explicit coordinates. Visualization options move
under `Visualization`, queries under `Data` targets, and time settings and the
UID move out of the dashboard builder.

`cmd/poc-v2/` holds four v2 dashboards that run on prd Grafana 13.2.2 through
the existing sidecar. They were written from Grafana's v2 JSON shape rather
than translated from the generator, so they hand-write panel IDs and grid
coordinates, inline shared thresholds, and drop the generator's constants,
helpers, and comments.

On a local Grafana 13.2.0, all 22 Classic dashboards convert to v2 without
failure and keep their panel, row, and variable counts (386, 113, and 55). The
largest v2 resource is DNS Overview at 139 KB. The file provider replaced a
Classic dashboard with a v2 resource of the same UID in place and back again.

The uptime rewrite replaced its Classic resource through the sandbox sidecar on
Grafana 13.3.0, preserving its UID and internal ID without duplication. All 16
queries succeeded; 14 returned no series because sandbox omits the external
scrapes. On local Grafana 13.2.0 with live data, Classic and v2 showed matching
status, availability, and series. AutoGrid rendered at 1440 px and reflowed to
one column at 375 px. The sandbox test configuration was restored afterward.

## Decision

Rewrite the generator on `dashboardv2` by translating the existing builders,
and retire the `dashboard` package.

- Translate each `cmd/generate/*.go` file into `cmd/poc-v2/`, keeping its
  queries, visualization settings, constants, and comments. Replace the current
  PoC code; keep only its variable-query builder.
- Lay translations out for v2 instead of reproducing Classic positions: rows
  of AutoGrid-placed panels, with per-row column and height settings and
  element names derived from panel titles. A row whose panels need different
  sizes is split. Port `helpers.go` to `dashboardv2` types.
- Accept the local variable-query builders and the managed-fields log line
  instead of waiting for upstream.
- Before cutover, compare Classic and v2 on local Grafana with live data:
  values, series, variable selection, colors, and layout at desktop and narrow
  widths. Keep automated structural validation and generated-file drift checks.
- Cut over dashboard by dashboard: the v2 resource keeps the Classic file name
  and UID, so ConfigMap names, bookmarks, and alert `dashboard_url` links do not
  change. Until then, translations are generated locally only.
- After the last cutover, replace `cmd/generate/` with the translated code.

## Alternatives rejected

- **Keep deferring.** The `dashboard` package is deprecated and the open
  triggers cost a few lines or a log line.
- **Extend the current PoC code.** It does not follow the generator's
  conventions, and hand-written coordinates do not scale to 386 panels.
- **Reproduce Classic positions with explicit grid coordinates.** Keeps the
  layout identical but needs a placement adapter, and Grafana stacks such grids
  into a single column on narrow screens. AutoGrid reflows by width and is the
  only layout that supports show/hide rules for panels.
- **Compare Classic and v2 JSON content.** Format normalization adds maintenance
  without verifying query execution or rendered behavior.
- **Generate Go from JSON with the SDK `DashboardConverter`.** Its output is
  untyped struct literals and maps with unconverted queries, not builder code.
- **Permanent `-v2` UIDs.** Doubles every dashboard and breaks alert links and
  bookmarks at the end.

## Consequences

- Generated JSON becomes `dashboard.grafana.app/v2` resources about twice the
  Classic size, within the 256 KB last-applied annotation limit that
  client-side apply imposes on each ConfigMap.
- The four PoC dashboards are removed from prd; translated dashboards appear
  there only at cutover.
- Local development needs Grafana 13.2 or later.
- `SA1019` deprecation warnings end with the `dashboard` package.
- ADR-0007 is superseded by this decision.
