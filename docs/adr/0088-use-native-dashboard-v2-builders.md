# ADR-0088: Use native dashboardv2 builders for dashboard structure

- **Status:** Accepted
- **Category:** Observability
- **Date:** 2026-11-13
- **Amended by:** [ADR-0089](0089-use-dashboard-v2-query-defaults.md)
- **Validated:** Native SDK definitions for Uptime and Disk Health, followed by
  all 22 dashboards, produce byte-identical JSON on 2026-11-13.
- **Related:** [ADR-0087](0087-migrate-grafana-dashboards-to-dashboard-v2.md)

## Context

The dashboard generator wraps the Foundation SDK with `newLayout` for panel
registration and placement, and `queries` for query wrapping and reference IDs.
These helpers reduce repetition but introduce a second construction API to
maintain as the SDK evolves. They hide the SDK's separation of elements from
layout and of panel queries from datasource queries.

## Decision

Use native `dashboardv2` builders for panel registration, rows, AutoGrid item
references, query groups, and query targets. Replace `newLayout`, automatic
title-derived element names, `queries`, and `queryWithRef`.

- Define element names as constants local to each dashboard builder and reuse
  them for registration and layout references. Preserve existing element keys,
  fixed panel IDs, query reference IDs, and dashboard UIDs.
- Retain small factories returning fresh SDK builders for shared presentation
  settings and datasource selection. Keep variable-query adapters only where
  the SDK lacks the required builders.
- Retain generated-dashboard validation and drift checks independently of the
  construction API. Require byte-identical generated JSON for this refactor.

This changes the construction convention, not ADR-0087's dashboardv2 resource
format or rows-and-AutoGrid layout choice. Update the dashboard README's
conventions when implementation changes.

## Alternatives considered

- **Keep the construction helpers.** Provides concise display-order definitions
  and registration checks, but requires maintaining an additional API when
  adopting SDK features.
- **Register panels through a separate map.** A map literal with constant keys
  rejects duplicate key values at compile time, but a map of builders requires
  a registration loop; a map of built elements requires individual build-error
  handling. Prefer the SDK's existing panel-registration API.
- **Inline all shared settings.** Removes helpers but duplicates visual and
  datasource conventions without clarifying dashboard structure.

## Consequences

Definitions become longer and explicitly separate panel content from placement.
Element identity no longer changes with display titles. Constants prevent
reference typos but do not guarantee distinct string values: repeated native
`Panel` registrations can overwrite an element, and post-build validation
cannot recover the overwritten registration. Migration comparisons must
therefore preserve the existing generated resources exactly.
