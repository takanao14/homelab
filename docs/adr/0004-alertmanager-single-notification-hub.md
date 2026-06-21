# ADR-0004: Alertmanager as the single notification hub (metrics + logs)

- **Status:** Accepted
- **Date:** 2026-06-21
- **Related:** [`docs/plans/proxmox-loki-log-collection.md`](../plans/proxmox-loki-log-collection.md) (Phase 5), [`k8s/monitoring/README.md`](../../k8s/monitoring/README.md), [ADR-0003](0003-proxmox-host-log-collection-via-rsyslog-forwarding.md)

## Context

Adding Proxmox **log** alerts (LogQL on Loki) raised the question of where
notifications are delivered. Key runtime facts about the prd cluster:

- `kube-prometheus-stack` already runs Alertmanager, but with no receiver/route —
  so the bundled **metric** alerts (`PrometheusRule`) were firing into the default
  null route, i.e. dormant.
- Grafana runs as a separate wrapper chart with no alert provisioning.
- Loki runs SingleBinary with an in-process ruler.
- `PrometheusRule` cannot evaluate LogQL against Loki.

A notification destination is therefore **not** a Loki-specific need: it is a
shared prerequisite that also revives the dormant metric alerts.

## Decision

Use **Alertmanager as the single delivery hub**:

- Metric alerts evaluate in Prometheus via `PrometheusRule`; log alerts evaluate
  in the Loki ruler via LogQL. **Both** route into the one Alertmanager.
- One routing tree, one contact point, one secret. The receiver is **Discord**
  (`discord_configs`), with the webhook URL delivered via OpenBao → ESO → Secret →
  `AlertmanagerConfig` (never in Git).
- Only `warning`/`critical` route to Discord; informational/internal alerts keep
  null routes.

## Alternatives considered

- **Split: metrics via Alertmanager, logs via Grafana-managed alerts.** *Rejected* —
  two notification configs and two secrets duplicate the delivery path.
- **Grafana-managed for everything.** *Rejected* — forces re-implementing every
  metric rule (including kube-prometheus-stack defaults) as Grafana-managed; a
  large migration for no maintainability gain.

This reverses an earlier deferral of the Loki ruler: that deferral assumed
Alertmanager integration was an added cost, but since Alertmanager is already
deployed with dormant metric alerts, the integration is a net benefit.

## Consequences

- Existing metric alerts revive with **zero migration**; all rules/routing stay
  declarative YAML in Git, no Grafana UI state.
- Rule evaluation stays in two engines (Prometheus + Loki ruler), but grouping,
  silencing, routing, the contact point, and the secret are consolidated.
- Accepted trade-off: loss of Grafana-managed conveniences (dashboard deep links,
  expression composition, explicit NoData/Error states).
