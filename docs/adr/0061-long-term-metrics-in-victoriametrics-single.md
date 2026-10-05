# ADR-0061: Keep long-term metrics in VictoriaMetrics single-node

- **Status:** Accepted
- **Date:** 2026-10-03
- **Related:** [ADR-0016](0016-cluster-label-via-default-scrape-class.md),
  [ADR-0018](0018-seaweedfs-data-on-usb-ssd-directory-storage.md)

## Context

prd keeps 30 days of metrics in a single Prometheus on node-local
`openebs-hostpath` storage. 180 days of history is wanted, and dashboards must
stay usable over 90–180 day ranges. At about 150k active series and
5.2k samples/s, Prometheus uses about 15 GB per 30 days.

## Decision

Run VictoriaMetrics single-node (Community edition) in prd with
`retentionPeriod: 180d`. Prometheus keeps scraping, evaluating rules, and
alerting with its 30-day retention, and forwards every sample with
`remote_write`. Grafana gets a second Prometheus-type datasource for it;
Prometheus stays the default, and dashboards switch to VictoriaMetrics through
their `$datasource` variable for ranges beyond 30 days. Schedule it away from
the Prometheus pod so the most recent 30 days exist on two workers.

## Alternatives considered

- **Raise the Prometheus retention to 180d.** *Rejected* — long-range queries
  over the raw TSDB are slow, and all history stays on one worker.
- **Grafana Mimir on SeaweedFS S3.** *Rejected* — the official chart is
  microservices-only and defaults to a Kafka-backed ingest path. The only
  object store is the USB SSD from ADR-0018, so a USB disconnect would also
  interrupt metrics. Mimir's scale, HA, and multi-tenancy are not needed.
- **Replace Prometheus with the VictoriaMetrics operator stack.** *Rejected* —
  it would rebuild scrape and alert paths that already work.

## Consequences

- Downsampling, retention filters, vmbackupmanager, and LTS releases require
  Enterprise. History is kept at full resolution, and Renovate follows the
  latest Community release with manual review.
- Data is deleted in monthly partitions, so disk use reaches about retention
  plus one month. A bare `retentionPeriod` number means months.
- History older than 30 days exists only on the VictoriaMetrics node until
  `vmbackup` is added.
- prd Prometheus sets no external labels, because remote_write and remote
  read would add them to every series, including the external targets that
  ADR-0016 keeps cluster-less. VictoriaMetrics therefore holds the same labels
  as the local TSDB. Alerts get `cluster=prd` through alert relabeling only
  when the series has no `cluster` label.
- MetricsQL does not extrapolate `rate()`/`increase()` and looks back one step
  in range queries. With identical samples, short-range `rate` panels differ
  by about 13% per point (median), while 30-day views match within 0.3%.
  Alerts are unaffected.
