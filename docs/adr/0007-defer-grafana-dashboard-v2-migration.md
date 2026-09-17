# ADR-0007: Defer Grafana Dashboard v2 migration

- **Status:** Accepted
- **Date:** 2026-06-21
- **Last reviewed:** 2026-09-17
- **Review trigger:** Grafana fixes the Dashboard v2 managed-fields schema,
  the Foundation SDK supports Prometheus query variables without a custom
  builder, and a stable v2 API or migration guide is available.
- **Related:** `k8s/monitoring/dashboards/cmd/generate/`. The deferral plan (`docs/plans/grafana-dashboard-v2-migration.md`) has been removed now that the decision lives here; see git history.

## Context

Dashboards under `k8s/monitoring/dashboards/cmd/generate/` use the **classic**
`dashboard` package of the Grafana Foundation SDK (`v0.0.20`). The SDK marks
`dashboard.Dashboard` / `DashboardBuilder` / `NewDashboardBuilder` deprecated in
favor of `dashboardv2`, and staticcheck `SA1019` reports expected deprecated
usages.

However: the SDK's intro docs and primary examples still use the classic API, no
complete v2 migration guide exists, and the SDK itself is public preview.
Dashboard v2 is not a package rename — it splits panels, visualizations, queries,
variables, and layouts into separate resource objects.

The September 2026 review deployed separate v2 PoCs for Uptime, DHCP Leases,
and cert-manager through the production ConfigMap sidecar. Grafana 13.2.2
registered and rendered all three alongside their Classic dashboards. This
covered Prometheus and Loki queries, query variables, logs, tables, merged
queries, transformations, field overrides, and value mappings.

Every v2 resource also logged Grafana's upstream
[managedFields schema error](https://github.com/grafana/grafana/issues/128991).
The Foundation SDK does not expose the Prometheus variable-query shape, so the
PoCs require a local builder for that schema. Neither issue broke rendering,
but both make a repository-wide migration premature.

## Decision

**Do not migrate to Dashboard v2 yet.** Keep generating dashboards with the
classic Foundation SDK API. The classic dashboards build and generate
successfully today.

## Consequences

- The `SA1019` deprecation warnings are accepted/expected for now and should
  not be treated as actionable lint failures.
- Migration scope when revisited is large: 22 dashboards / 377 panels / 111 rows /
  55 variables, across Prometheus + Loki queries and multiple visualization types,
  plus transformations, value mappings, and field overrides.
- Keep the three v2 PoCs as regression coverage while Classic dashboards remain
  the production source of truth.
- Revisit when the review trigger is satisfied; supersede this ADR before
  migrating the remaining dashboards.
