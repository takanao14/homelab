# ADR-0048: Expose Vector internal metrics on service VMs

- **Status:** Accepted
- **Date:** 2026-09-11
- **Related:** [ADR-0047](0047-service-vm-logs-via-vector-journald-agent.md),
  [ADR-0016](0016-cluster-label-via-default-scrape-class.md)

## Context

Loki confirms that a VM produced logs, but it cannot explain an absence when
Vector itself is stopped, buffering, discarding events, or failing to deliver
to the Loki sink. Vector's own logs cannot close this gap because a broken sink
may also prevent those diagnostics from reaching Loki.

## Decision

Service VMs expose Vector's `internal_metrics` through its Prometheus exporter
on TCP 9598, bound to the VM's managed inventory address. Prometheus will scrape
the endpoints as external LAN targets and provide durable metrics and alerts.
The metrics omit Vector's `host` tag and use the scrape target's stable
`instance` label instead.

The Vector observability API remains disabled. It has no authentication and is
useful for interactive `top` and `tap` diagnostics, not durable monitoring.

## Consequences

- Prometheus can distinguish process reachability, Loki sink errors,
  unintentional discards, event flow, and buffer pressure.
- Each service VM gains an unauthenticated metrics listener on its managed LAN
  address, consistent with the existing node-exporter exposure.
- The additional source and sink add a small fixed resource cost to Vector.
- Hosts with a custom `vector_config` do not inherit the standard metrics
  pipeline unless they opt in explicitly.
