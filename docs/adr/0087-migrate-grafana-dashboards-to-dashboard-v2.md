# ADR-0087: Migrate Grafana dashboards to Dashboard v2

- **Status:** Proposed
- **Category:** Observability
- **Date:** 2026-11-11
- **Review trigger:** The sandbox check that a v2 resource replaces a Classic
  dashboard with the same UID through the ConfigMap sidecar has a result.
- **Related:** [ADR-0007](0007-defer-grafana-dashboard-v2-migration.md),
  [`k8s/monitoring/dashboards`](../../k8s/monitoring/dashboards/README.md)

## Context

ADR-0007 deferred the migration until Grafana fixed the Dashboard v2
managed-fields schema error, the Foundation SDK supported Prometheus query
variables, and a stable v2 API existed. It required a superseding ADR before the
remaining dashboards move.

Since then four v2 PoCs (Uptime, DHCP Leases, cert-manager, and 20 of the 43
DNS Overview panels) have been registered on prd Grafana 13.2.2 as
`dashboard.grafana.app/v2` resources through the existing sidecar. They cover
every panel type in use except `gauge`, both datasources, Prometheus query
variables, transformations, overrides, and value mappings.

Two of the three triggers are still open. Foundation SDK `v0.0.22`, the latest
release, has no Prometheus variable-query builder, so the PoCs keep a local one
of about 15 lines. The managed-fields error is logged without affecting
registration or rendering; upstream redirected it to
[grafana-app-sdk#1485](https://github.com/grafana/grafana-app-sdk/issues/1485),
which is open with no fix or maintainer response as of 2026-11-11. The third
trigger is met: the v2 schema became generally available in Grafana 13.0.

A local Grafana 13.2.0 file provider replaced a Classic dashboard with a v2
resource of the same UID in place and back again: one dashboard, an incremented
generation, no conflict, and a served v2 spec equal to the source apart from
two dropped empty `custom` objects. The ConfigMap sidecar path in sandbox is
still untested.

The Classic generator covers 22 dashboards, 386 panels, 109 rows, and 55
variables. It uses no repeats, collapsed rows, links, annotations, or expression
queries, so the remaining unknowns are narrow: `gauge` panels, Loki query
variables, a custom variable, hidden variables, and panel minimum intervals.

## Decision

Migrate all dashboards to Dashboard v2 and retire the Classic generator.

- Accept the local variable-query builders and the managed-fields log line as
  known costs instead of waiting for upstream.
- Replace each dashboard in place: the v2 resource keeps the Classic file name
  and UID, so ConfigMap names, bookmarks, and alert `dashboard_url` annotations
  do not change.
- Gate every replacement on an automated parity check between the v2 resource
  and the Classic JSON it replaces. Rendering is reviewed by eye; queries,
  units, thresholds, mappings, overrides, transformations, variables, and row
  membership are not.
- Build shared v2 helpers and automatic grid placement before porting further
  dashboards. The PoCs hand-write panel IDs and coordinates, which does not
  scale to 386 panels and contradicts the dashboard helper conventions.
- Migrate in batches ordered by feature risk, deleting each Classic definition
  in the change that replaces it.

If the in-place replacement check fails, cut over through temporary parallel
UIDs instead and revise this record before accepting it.

## Alternatives rejected

- **Keep deferring.** The Classic SDK API is deprecated and the open triggers
  are cosmetic or cost a few lines; four PoCs already run in production.
- **Permanent `-v2` UIDs.** Doubles every dashboard during the migration and
  breaks alert links and bookmarks at the end.
- **Single cutover of all 22 dashboards.** One unreviewable change with no
  per-dashboard rollback.

## Consequences

- Local development needs Grafana 13.2 or later; the Classic compose image is
  replaced.
- v2 resources are roughly twice the size of Classic JSON (Uptime: 22 KB to
  44 KB). The `dashboards` application uses client-side apply, so each
  ConfigMap is bound by the 256 KB last-applied annotation limit, not the 1 MiB
  ConfigMap limit; the largest dashboard is estimated at about 160 KB.
- The parity check and the committed Classic JSON disappear dashboard by
  dashboard; structural validation of v2 resources remains.
- `SA1019` deprecation warnings end with the Classic generator.
- On acceptance, mark ADR-0007 `Superseded by ADR-0087`.
