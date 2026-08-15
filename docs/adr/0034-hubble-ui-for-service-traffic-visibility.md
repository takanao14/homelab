# ADR-0034: Visualize service-to-service traffic with Hubble UI

- **Status:** Proposed
- **Date:** 2026-08-14
- **Related:** [ADR-0009](0009-longhorn-ui-exposed-through-authenticated-gateway-route.md),
  [ADR-0011](0011-cilium-gateway-to-envoy-gateway-migration.md),
  [ADR-0015](0015-headlamp-per-cluster-in-cluster-deployment.md),
  [ADR-0027](0027-gpu-workload-switching-web-ui.md),
  [`k0s/values/cilium.yaml.gotmpl`](../../k0s/values/cilium.yaml.gotmpl),
  [`k8s/headlamp`](../../k8s/headlamp/README.md),
  [`docs/service-routing.md`](../service-routing.md)

## Context

Several views onto the clusters already exist, and none of them show traffic
between services:

- **Headlamp**, per cluster (ADR-0015). Its Map view draws *declarative*
  relationships — owner references, Service selectors, mounted ConfigMaps and
  Secrets — not observed flows.
- **Grafana + Prometheus**. Time series, including the Cilium and Hubble L3/L4
  metrics already exported (`dns`, `drop`, `tcp`, `flow`, `icmp`,
  `port-distribution`).
- **Homepage**. A static tile list of service endpoints.
- **`docs/service-routing.md`**. A hand-maintained mermaid diagram of the three
  ingress paths, accurate only as long as someone updates it.

The gap is a service dependency graph derived from real traffic: which workload
actually talks to which, and which flows are dropped. Cilium 1.19.5 is already
the CNI and Hubble is already collecting the flow data that would answer this;
only `hubble.relay` and `hubble.ui` are left at their chart defaults, so no
Hubble Pod runs in `kube-system` today.

## Decision

*Proposed, not yet adopted.*

Enable Hubble relay and UI in the existing Cilium release to obtain the service
map, and keep Headlamp's Map view as the view of declarative relationships. Do
not introduce a third, standalone topology UI.

One point is deliberately left open until this moves to `Accepted`: **how the UI
is reached.** Cilium's chart emits only an `Ingress`, while ingress here is
Gateway API through Envoy Gateway (ADR-0011). Cilium is configured in the `k0s/`
bootstrap layer and Envoy Gateway in the `k8s/` Argo CD layer, so publishing the
UI splits one component across both layers — a small `k8s/hubble-ui` chart
carrying the `HTTPRoute` and a `SecurityPolicy`, exactly the shape ADR-0009 used
for the Longhorn UI. The alternative is to leave the UI unpublished and reach it
with `kubectl port-forward`, keeping the component within a single layer at the
cost of convenience.

## Alternatives considered

- **Grafana + Alloy `beyla.ebpf`.** *Rejected for now.* It would add L7 RED and
  service-graph metrics on top of the Alloy DaemonSet that already runs, which
  is more than Hubble's L3/L4 map offers. But Beyla requires `hostPID: true` and
  an Unconfined AppArmor profile on that DaemonSet, widening the privileges of
  the collector that scrapes the whole fleet. Worth revisiting as its own ADR if
  L7 latency and error rates — not topology — become the requirement.
- **Kiali.** *Rejected.* It reads an Istio mesh. Ingress here is Envoy Gateway
  with no mesh (ADR-0011), and adopting Istio to gain a graph inverts the cost.
- **KubeView.** *Rejected.* Its namespace resource map largely duplicates
  Headlamp's Map view, in exchange for another Argo CD application and a thinner
  maintenance story than a Kubernetes SIG project.
- **kube-ops-view.** *Rejected.* Archived on GitHub in October 2020. Its
  node-and-pod placement view also answers a different question than service
  dependencies.
- **Tempo with span-derived service graphs.** *Rejected.* Requires standing up a
  tracing backend and instrumenting workloads — disproportionate when the flow
  data is already being collected at the CNI.
- **Status quo — Grafana panels over the existing Hubble metrics.** *Rejected as
  insufficient.* The metrics aggregate flows into time series; they cannot
  reconstruct which identity talked to which, which is the question being asked.

## Consequences

- Two additional Pods in `kube-system` (`hubble-relay`, `hubble-ui`). The agents
  already collect the flow data, so per-node cost is unchanged.
- The map covers L3/L4 only. L7 detail (HTTP method, path, status) requires
  Envoy redirection per workload and is explicitly not enabled here.
- Hubble UI carries no authentication of its own. If published through the
  shared gateway it needs a `SecurityPolicy` in front of it, as `gpu-switch`
  does (ADR-0027); a read-only view of every flow in the cluster is not a LAN
  free-for-all.
- Flow history is bounded by `eventBufferCapacity` (8191 events). The UI shows a
  live window, not a queryable history — it complements Loki rather than
  replacing a log search.
- If the `HTTPRoute` path is chosen, Cilium becomes a component whose
  configuration lives in `k0s/` and whose route lives in `k8s/`. After a cluster
  rebuild the UI returns only once Argo CD syncs, while the CNI itself is up
  much earlier.
