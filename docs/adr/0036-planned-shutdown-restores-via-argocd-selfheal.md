# ADR-0036: Planned-shutdown restore relies on Argo CD self-heal, not a replica snapshot

- **Status:** Accepted
- **Date:** 2026-08-25
- **Related:** [ADR-0027](0027-gpu-workload-switching-web-ui.md),
  [ADR-0032](0032-reboot-and-planned-shutdown-orchestration.md),
  [`ansible/playbooks/tasks/k8s_workload_stop.yaml`](../../ansible/playbooks/tasks/k8s_workload_stop.yaml),
  [`ansible/playbooks/tasks/k8s_workload_start.yaml`](../../ansible/playbooks/tasks/k8s_workload_start.yaml)

## Context

ADR-0032 rejected letting Argo CD restore replicas after a planned shutdown,
because a Helm chart that omits `replicas` gives Argo CD nothing to diff
against — the workload silently stays at zero. It snapshotted every scaled
workload's live `.spec.replicas` to a file outside the repo before scaling
down, and restored from that file on startup.

Auditing this repository's own Helm charts found the gap was narrow: of nine
custom Deployment templates, only `k8s/externalDNS/chart/templates/deployment.yaml`
omitted `replicas` (no custom StatefulSet templates exist).

Upstream dependency charts were rendered directly (`helm template` against
each pinned `repoURL`/`chart`/`targetRevision` in
`k8s/argocd/apps/values.yaml`, with this repo's own values layered on) to
check the same thing for open-webui, reloader, the argo-helm chart argocd
manages itself with, kube-prometheus-stack, and grafana. Every Deployment and
StatefulSet rendered `replicas` explicitly, with one exception:
open-webui's own `templates/websocket-redis.yaml` (the bundled Redis backing
`websocket.manager: redis`) never sets `replicas` and exposes no
`replicaCount` value to fix it — an upstream gap, not something this repo's
values can override. Since `open-webui` in this repo already runs a single
`replicas: 1` StatefulSet, and the chart's own docs note the built-in
in-memory websocket manager only works at replica count 1, the fix was to
stop deploying that Redis at all: `k8s/open-webui/values.yaml` now sets
`websocket.manager: ""` and `websocket.redis.enabled: false`, removing the
Deployment (and its bug) rather than working around it.

Separately, `ollama`, `comfyui`, `vllm`, and `lemonade-server` are
GPU-switchable (ADR-0027): their `Application`s carry
`ignoreDifferences` on `/spec/replicas`, so Git's declared `replicaCount: 0`
is never what's actually running, by design. Restoring "whatever Git
declares" for these four is fine — they are switched on demand, not expected
to resume automatically — but this only works because ADR-0027 already ships
a custom health Lua (`k8s/argocd/values-common.yaml`) that reports a
scaled-to-zero GPU workload as `Healthy` rather than `Degraded`. That means a
single uniform wait condition — every Argo CD `Application` is `Synced` and
`Healthy` — works for GPU and non-GPU workloads alike, with no separate
exclusion list.

## Decision

**Fix the one real gap (`external-dns`), then replace the file-based restore
with: resume the Argo CD application-controller, and wait for every
`Application` to report `Synced`/`Healthy`.**

This supersedes, in part, ADR-0032's rejected alternative "Let Argo CD
restore replicas after a shutdown" — ADR-0032's status is left `Accepted`
and not edited, per this directory's append-only convention; only the
planned-shutdown restore mechanism changes. Its rolling-reboot strategy,
inventory-driven ordering, and hypervisor shutdown sequencing are unaffected.

`k8s/externalDNS/chart/templates/deployment.yaml` now sets `replicas: 1`
directly (no values field — the value never varies, so a chart knob would be
unused surface).

`ansible/playbooks/tasks/k8s_workload_stop.yaml` still scales every
Deployment/StatefulSet across `k8s_shutdown_namespaces` to zero and waits for
drain — the "no eviction target" reasoning behind scale-to-zero is unchanged
— but no longer records `.spec.replicas` or writes a snapshot file.

`ansible/playbooks/tasks/k8s_workload_start.yaml` no longer reads a snapshot.
It first restores the minimum Argo CD bootstrap set (Redis, repo-server, and
application-controller) and waits for those resources to become ready. It then
requests a hard refresh of every `Application` before polling until all report
`Synced Healthy`; requiring the refresh to be consumed prevents stale
pre-shutdown status from satisfying the wait. Argo CD's own self-heal (already
`selfHeal: true` on every `Application` in this repo) restores the remaining
components and workloads.

`k8s_shutdown_state_dir` is removed from
`ansible/inventories/homelab/group_vars/k8s_controller.yaml` as dead
configuration.

## Alternatives considered

- **Keep the snapshot, only fix `external-dns`.** Removes today's one known
  gap but keeps carrying external runtime state (`~/.local/state/homelab/shutdown`)
  and the risk ADR-0032 already flagged: "losing it means restoring replica
  counts by hand." *Rejected* — the point of this change is to stop depending
  on that file at all.
- **Exclude GPU-switchable namespaces from the health-wait explicitly.**
  Unnecessary: ADR-0027's health Lua already reports their scaled-to-zero
  state as `Healthy`, so the uniform wait condition already handles them
  correctly without a namespace list to maintain.

## Consequences

- A planned shutdown/startup no longer depends on any file outside the repo.
  All restore information lives in Git, matching how the rest of this
  cluster's desired state is managed.
- GPU-switchable workloads (`ollama`, `comfyui`, `vllm`, `lemonade-server`)
  no longer resume whatever was running before an outage — they come back at
  Git's declared zero and must be switched on again with `gpu-switch.sh` or
  the gpu-switch UI, same as any other time. This was confirmed acceptable:
  these workloads are already switched on demand, not expected to survive a
  reboot running.
- Any future chart added under `k8s/` that omits an explicit `replicas` (or
  whose upstream chart does) silently reproduces the original failure mode:
  it will not resume after a planned shutdown, with no error — Argo CD simply
  has nothing to reconcile it against. This is the same risk ADR-0032 named
  for the snapshot approach, moved to chart-review time instead of
  shutdown/startup time.
- Verified by rendering every upstream dependency chart in scope (open-webui,
  reloader, argo-helm, kube-prometheus-stack, grafana) at the pinned versions
  in `k8s/argocd/apps/values.yaml`, with this repo's own values layered on —
  every Deployment/StatefulSet renders an explicit `replicas` except the
  open-webui Redis fixed above. Re-run this check
  (`helm template <chart> --repo <repoURL> --version <targetRevision> -f
  <this repo's values>`, grep rendered `Deployment`/`StatefulSet` objects for
  `spec.replicas`) whenever a chart version bumps or a new one is added to
  `k8s_shutdown_namespaces`.
