# ADR-0089: Use dashboardv2 query defaults without legacy compatibility

- **Status:** Accepted
- **Category:** Observability
- **Date:** 2026-11-14
- **Related:** [ADR-0088](0088-use-native-dashboard-v2-builders.md)

## Context

ADR-0088 preserved generated JSON exactly while removing construction wrappers.
That constraint retained explicit query IDs and a custom numbering function even
where nothing references those IDs. Designing the dashboards as native v2
resources makes those compatibility-only settings unnecessary.

## Decision

Let the SDK assign panel-query reference IDs unless another component references
the query. Keep meaningful explicit IDs for query-based overrides, transformations,
and expressions. Remove custom query numbering and its dedicated tests.

Set referenced IDs only on `TargetBuilder`, not the nested datasource query.
Grafana v13.3.0's
[`panelQueryKindToSceneQuery`](https://github.com/grafana/grafana/blob/v13.3.0/public/app/features/dashboard-scene/serialization/layoutSerializers/utils.ts#L430-L441)
copies the outer ID into the runtime query before spreading datasource options;
a nested ID is redundant and can override it. Variable-query adapters remain
scoped to the shapes required by the datasource editors.

Validate nonempty, unique query IDs and the existence of query references used by
field overrides. Generated-file drift checks remain required, but equality with
the previous dashboard definitions is no longer an acceptance criterion. Verify
query execution, transformations, overrides, variables, and rendering when runtime
behavior changes. Numeric panel IDs and element keys are separate concerns and
are not removed by this decision.

## Consequences

Unreferenced query IDs follow the SDK's numbering and can change when queries
are reordered. Generated JSON changes without requiring compatibility mappings.
Explicit references remain readable and independent of automatic numbering.
