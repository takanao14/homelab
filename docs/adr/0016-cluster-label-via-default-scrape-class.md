# ADR-0016: Cluster label via default scrape class, asymmetric per environment

- **Status:** Accepted
- **Date:** 2026-07-09 (implemented and verified 2026-07-04)
- **Related:** [`k8s/monitoring/README.md`](../../k8s/monitoring/README.md),
  [ADR-0004](0004-alertmanager-single-notification-hub.md). The implementation
  plan (`prometheus-scrape-gaps.md` in the private plans repo) has been removed
  now that the rollout is complete.

## Context

prd Prometheus is the central store; dev runs a PrometheusAgent that
remote-writes into it. dev series get `cluster=dev` cheaply via
`externalLabels` at remote_write time, but externalLabels do **not** apply to
a Prometheus instance's own local TSDB queries — so every prd (and sandbox)
in-cluster scrape resource had to repeat a `cluster: prd` target relabeling
(values overrides, kubelet endpoints, cilium ServiceMonitors, …). Pure
boilerplate, and easy to forget on new ServiceMonitors.

## Decision

Stamp the cluster label with a **default Prometheus Operator scrapeClass**
per full-server environment, and keep dev on remote_write externalLabels:

- prd / sandbox: `scrapeClasses` with `default: true` (`cluster-prd` /
  `cluster-sandbox` in `values/prometheus*.yaml`) applying a
  `targetLabel: cluster` relabeling to every scrape resource.
- A named opt-out class `external` (no relabelings) for targets that are not
  cluster workloads: external LAN scrape resources (ScrapeConfigs and Probes),
  and resources that set their own per-target `cluster` labels
  (`control-plane-node-exporter`).
- Use `relabelings` (target relabeling), **not** `metricRelabelings`, so
  synthetic series (`up`, `scrape_*`) also carry the label — a past
  `KubeletDown` false-fire was caused by exactly this gap.

## Alternatives considered

- **Per-resource relabelings (status quo ante).** *Rejected* — boilerplate on
  every resource and a silent-failure mode when forgotten.
- **Symmetric remote_write aggregation** (prd also becomes an agent; both
  clusters write to a dedicated aggregator, so the label always arrives via
  externalLabels). *Rejected* — instance count grows 2→3 on homelab hardware;
  agent mode cannot evaluate rules, so the entire prd rule set and
  Alertmanager move to the aggregator, re-opening the remote-write pitfall
  class already debugged once (up-label false fires, cAdvisor out-of-order);
  external LAN targets still need one owner, so the asymmetry never fully
  disappears. Re-evaluate if a third permanent cluster appears, alerting is
  reworked anyway, or prd storage/query pressure forces a split (then compare
  Thanos/Mimir too).

## Consequences

- New in-cluster ServiceMonitors/PodMonitors need no cluster relabeling in
  any environment.
- Every external scrape resource **must** set `scrapeClass: external`, or the
  default class stamps the cluster label onto LAN hosts and leaks them into
  `cluster=~"$cluster"` dashboard queries.
- The cluster-label mechanism intentionally differs per environment
  (scrapeClass vs remote_write); the paired prd/dev scrape templates document
  this in their comments.
