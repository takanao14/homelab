# ADR-0024: Shared Proxmox node inventory for monitoring

- **Status:** Accepted
- **Date:** 2026-07-13
- **Related:** [ADR-0014](0014-argocd-app-of-apps-shared-helm-chart.md),
  [ADR-0016](0016-cluster-label-via-default-scrape-class.md)

## Context

Adding a Proxmox hypervisor required editing four independent places under
`k8s/monitoring`:

1. `values/node-exporter-external.yaml` — node-exporter scrape target
2. `values/blackbox-exporter-external.yaml` — ICMP probe target plus the
   `<node>-amt` AMT probe target
3. `charts/loki/templates/proxmox-alerting-rules.yaml` — a hardcoded
   `{host=~"pve|node1|node2|node3"}` selector repeated in all seven LogQL
   alert rules
4. `dashboards/cmd/generate/proxmox_logs.go` — a `proxmoxHosts` constant baked
   into the generated `proxmox-logs` dashboard JSON

The duplication had already caused silent drift: when node4 was added
(2026-07-05) only the two exporter values files were updated, so none of the
seven Proxmox log alerts (OOM, storage, backup, quorum, HA, AppArmor, service
errors) ever fired for node4, and the proxmox-logs dashboard excluded it.
node5 (added to `tf/` and Ansible on 2026-07-13) was missing from all four
places.

## Decision

Introduce `k8s/monitoring/values/proxmox-nodes.yaml` as the single source of
truth for the hypervisor list (`proxmoxNodes`: `name`, `ip`, optional
`amtIp`), and derive every consumer from it:

- The app-of-apps chart appends the shared file to the `valueFiles` of the
  `node-exporter-external`, `blackbox-exporter-external`, and `loki`
  Applications (multiple value files merge; the key is disjoint).
- `node-exporter-external` renders one scrape target per node; per-host
  extras (Raspberry Pis, stateful guests) stay in its own values file.
- `blackbox-exporter-external` renders one ICMP probe per node IP and one per
  `amtIp` when set (labeled `<name>-amt`); network devices stay in its own
  values file.
- The Loki rules template builds the host regex with
  `join "|" (pluck names)`, replacing the seven hardcoded selectors.
- The dashboard generator reads the same YAML at `make generate` time
  (`cmd/generate/proxmox_nodes.go`), replacing the Go constant.

Adding a hypervisor is now one entry in `values/proxmox-nodes.yaml` plus a
dashboard regeneration (`make generate` in `dashboards/`, enforced by
`make check`). The same change also fixes the node4/node5 gaps.

## Alternatives considered

- **Generate the values files from one canonical file** — keeps charts free
  of derivation logic, but regeneration can be forgotten (the exact failure
  mode being fixed) unless CI enforcement is added.
- **Use the Ansible inventory as the source** — truly single-source across
  IaC layers, but couples `k8s/` to `ansible/` paths and would push
  monitoring-only attributes (AMT IPs) into the server inventory. Remains a
  possible evolution: the shared values file could later be generated from
  the inventory without touching the chart-side wiring.
- **Keep the lists and add a CI consistency check** — minimal change, but
  adding a node would still touch four places.
- **Dynamic discovery (Proxmox API / NetBox / DNS SD)** — rejected: the
  scrape design intentionally avoids runtime dependencies (targets are fixed
  IPs so monitoring works when DNS or other services are down), and a
  discovery service would invert that.

## Consequences

- The hypervisor list exists once; exporters, probes, log alerts, and the
  proxmox-logs dashboard cannot drift from each other.
- Scrape/probe templates gained small loops and the Loki template gained a
  regex derivation; chart values files now describe only non-hypervisor
  targets.
- The dashboard generator gained a `gopkg.in/yaml.v3` dependency and a
  build-time file read (`../values/proxmox-nodes.yaml`, relative to the
  `dashboards/` working directory assumed by the Makefile).
- Rendering the affected charts standalone requires passing
  `values/proxmox-nodes.yaml` in addition to the component values file; the
  Loki template fails fast with a clear error if the inventory is missing
  while `proxmoxAlerts` is enabled.
- node4 and node5 are now covered by all scrapes, probes, and log alerts;
  node5's AMT probe is pending its `amtIp` entry.

## Addendum (2026-07-13): shared DNS recursor inventory

A survey of the remaining target lists applied the same criterion — extract
only when the same host set is enumerated independently by multiple
consumers:

- **DNS recursors (dist1/dist2)** were listed three times: `dnsdist` metrics
  targets, and the blackbox `dns_external` and `dns_internal` probes (same
  set, different probe module). Extracted to
  `values/dns-recursors.yaml` (`dnsRecursors`), appended to the `dnsdist` and
  `blackbox-exporter-external` valueFiles; both DNS probe templates and the
  dnsdist scrape template derive from it.
- **Network devices (bgw1/c1200)** also appear three times (blackbox ICMP,
  snmp-exporter values, network dashboard variable), but the snmp-exporter
  `Probe` is rendered by the upstream chart from plain values, so a shared
  list cannot be looped into it without first moving the Probe into a local
  chart. Deferred until SNMP targets actually grow.
- **rpi3/rpi4** (two consumers, two stable hosts) and all single-consumer
  lists (pdns-auth, gpuvm, control-plane, stateful guests) stay inline: for a
  single consumer the values file already is the inventory, and extraction
  would only add indirection. A single all-hosts inventory with role flags
  was rejected for the same reason — inventories are per fleet that scales
  together.
