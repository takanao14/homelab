# ADR-0019: Merge the GPU worker into prd and retire the dev cluster

- **Status:** Accepted (migration in progress; runbook in the private plans
  repo, `dev-cluster-retirement.md`)
- **Date:** 2026-07-09
- **Related:** [ADR-0011](0011-cilium-gateway-to-envoy-gateway-migration.md),
  [ADR-0014](0014-argocd-app-of-apps-shared-helm-chart.md),
  [ADR-0016](0016-cluster-label-via-default-scrape-class.md),
  [`k0s/README.md`](../../k0s/README.md), [`k8s/README.md`](../../k8s/README.md)

## Context

The dev cluster (cp1 + worker1 on pve, plus the GPU worker VM `gpuvm`,
RX 9060 XT passthrough) exists alongside prd (cp1 + worker1 on node1). An
audit of `k8s/argocd/dev/apps-values.yaml` showed that everything unique to
dev is the GPU/LLM workload set — ollama, comfyui, lemonade-server,
open-webui — plus meshcentral. The other nine enabled apps (argocd,
cert-manager, envoy-gateway, eso, external-dns, gateway, headlamp,
monitoring agent, reloader) are duplicates of the prd platform stack.

The original justification for a persistent second cluster — a place to
validate platform changes before prd — has migrated to sandbox, which is
rebuilt on demand and already validated the Envoy Gateway migration
end-to-end (ADR-0011). Meanwhile the split carries a measurable ongoing tax:
paired prd/dev scrape templates, the `k8s/dev-monitoring` PrometheusAgent
with its remote_write pitfall class, double k0s upgrades and rebuild
procedures, a second OpenBao auth mount, a second wildcard certificate and
DNS zone, and 10 vCPU / 12 GiB of pve capacity for the dev control plane and
worker.

Workload isolation does not require a separate cluster: the GPU node already
carries `gpu=amd:NoSchedule`, and the GPU charts already pin themselves with
`nodeSelector: gpu=amd` + toleration + `amd.com/gpu` requests, so the same
mutual isolation holds inside prd.

## Decision

Join `gpuvm` (192.168.20.22, on pve) to the prd cluster as its GPU worker
and retire the dev cluster entirely:

- `k0s/env/prd.sh` gains `K0S_GPU_WORKER_ADDRESSES=192.168.20.22`; the
  existing k0s tooling applies the AMD device plugin, `gpu=amd` label,
  taint, and CoreDNS toleration unchanged. The VM keeps its 192.168.20.22
  address — prd and the 20.x subnet route to each other on the same LAN, and
  pve has no net30 bridge.
- The GPU/LLM apps and meshcentral move to prd by enabling them in
  `k8s/argocd/prd/apps-values.yaml` and renaming hostnames to
  `*.prd.butaco.net`. Chart directories, namespaces, and the
  `gpu-switch.sh` exclusive-workload model are unchanged (only its expected
  kube context changes).
- `k8s/dev-monitoring`, the dev overlays (cert-manager, externalDNS,
  gateway, headlamp, eso, argocd), the dev OpenBao auth mount, and the dev
  cluster VMs are removed. gpuvm's node metrics are scraped by the prd
  node-exporter DaemonSet; the external GPU-exporter ScrapeConfig
  (`amd-gpu-external`) is unaffected.

## Alternatives considered

- **Keep the split (status quo).** *Rejected* — the only remaining benefit
  is a persistent pre-prod cluster for k0s/Cilium upgrades, and sandbox
  covers that on demand (`K0S_STORAGE_PROVIDER=openebs` reproduces the prd
  storage configuration). The duplication tax is continuous; the benefit is
  occasional.
- **Keep serving the apps at `*.dev.butaco.net` from prd.** *Rejected* — it
  would need an extra Gateway listener and a second wildcard certificate for
  naming continuity only.
- **Run the LLM stack directly on the VM without Kubernetes.** *Rejected* —
  it would abandon GitOps management and the established chart/taint/switch
  machinery for no operational gain.

## Consequences

- One platform stack to operate, upgrade, and re-register with OpenBao. The
  remote_write agent pattern and the prd/dev paired-template convention
  disappear; ADR-0016's dev externalLabels mechanism becomes historical
  (prd/sandbox scrape classes remain).
- prd spans two Proxmox hosts (node1 + pve). ROCm/kernel maintenance on the
  GPU node happens inside prd, bounded by the taint: only the GPU apps stop,
  exactly as they do today when gpuvm is maintained.
- The prd cluster now spans two L2 segments while its LB pool lives on one
  (30.128–254). Cilium L2 announcements do not verify that a VIP belongs to
  the announcing node's subnet, so the intended rule — each worker answers
  only for the L2 segment it sits on — is encoded explicitly: workers get
  an L2-segment label derived from their IP at k0sctl-config generation,
  and `CiliumL2AnnouncementPolicy` selects only nodes whose segment matches
  the pool's. Pod networking itself is unaffected: Cilium runs in tunnel
  (VXLAN) mode, which works over routed L3.
- `cluster=dev` series end in Prometheus; dashboards keep them as history
  until retention expires. The dev-cp1 control-plane scrape targets are
  removed from `control-plane-metrics`.
- Ollama/ComfyUI model caches (openebs-hostpath PVCs) are recreated on the
  prd side; models re-download.
- Sandbox becomes the only pre-prod validation path; platform upgrades that
  previously soaked on dev should spin sandbox first when risk warrants it.
- 10 vCPU / 12 GiB are freed on pve (dev cp1 + worker1).
