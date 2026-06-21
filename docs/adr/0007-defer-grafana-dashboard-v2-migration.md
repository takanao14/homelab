# ADR-0007: Defer Grafana Dashboard v2 migration

- **Status:** Accepted
- **Date:** 2026-06-21
- **Related:** `k8s/monitoring/dashboards/cmd/generate/`. The deferral plan (`docs/plans/grafana-dashboard-v2-migration.md`) has been removed now that the decision lives here; see git history.

## Context

Dashboards under `k8s/monitoring/dashboards/cmd/generate/` use the **classic**
`dashboard` package of the Grafana Foundation SDK (`v0.0.18`). The SDK marks
`dashboard.Dashboard` / `DashboardBuilder` / `NewDashboardBuilder` deprecated in
favor of `dashboardv2`, and staticcheck `SA1019` reports 27 expected deprecated
usages.

However: the SDK's intro docs and primary examples still use the classic API, no
complete v2 migration guide exists, and the SDK itself is public preview.
Dashboard v2 is not a package rename — it splits panels, visualizations, queries,
variables, and layouts into separate resource objects.

## Decision

**Do not migrate to Dashboard v2 yet.** Keep generating dashboards with the
classic Foundation SDK API. The classic dashboards build and generate
successfully today.

## Consequences

- The 27 `SA1019` deprecation warnings are accepted/expected for now and should
  not be treated as actionable lint failures.
- Migration scope when revisited is large: ~13 dashboards / 175 panels / 52 rows /
  31 variables, across Prometheus + Loki queries and multiple visualization types,
  plus transformations, value mappings, and field overrides.
- Revisit when the Foundation SDK ships a stable v2 API and a migration guide;
  supersede this ADR at that point.
