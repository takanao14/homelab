# ADR-0028: lemonade-server runs on the Vulkan backend, not ROCm, on RDNA4

- **Status:** Accepted
- **Date:** 2026-07-27
- **Related:** [ADR-0019](0019-merge-gpu-worker-into-prd-retire-dev-cluster.md),
  [ADR-0027](0027-gpu-workload-switching-web-ui.md),
  [`k8s/lemonade-server/README.md`](../../k8s/lemonade-server/README.md),
  [`k8s/comfyui/README.md`](../../k8s/comfyui/README.md)

## Context

The prd GPU worker has one AMD RX 9060 XT — RDNA4, ISA `gfx1200`. lemonade
v10.8.0's built-in `llamacpp:rocm` backend refuses it outright:

```
llamacpp   rocm   unsupported   Unsupported GPU: gfx1200
```

This is an upstream defect, not a misconfiguration. lemonade detects the card by
its KFD-derived name (`AMD Radeon Graphics (RADV GFX1200)`), extracts the raw
token `gfx1200`, and then compares it for **exact equality** against its recipe
table, which lists RDNA4 only as the wildcard `gfx120X`. Cards detected by
marketing name normalise to `gfx120X` and pass; cards detected by ISA name never
can. Switching the ROCm channel between `stable` and `nightly` does not change
it, and the chart's `LEMONADE_LLAMACPP=rocm` environment variable turned out to
be a no-op — v10.8.0's source never reads it.

Two routes get past the gate, and they differ in kind:

- **`llamacpp:system`** — its allowed-AMD-family set is *empty*, which the
  constraint check treats as "allow all", so the gate is skipped. In exchange
  lemonade will not fetch a backend for you: the image must supply a
  ROCm-enabled `llama-server` on `$PATH` plus a discoverable `libggml-hip.so`.
  That means a custom image.
- **`llamacpp:vulkan`** — no GPU-family gate at all, and lemonade downloads the
  matching upstream llama.cpp Vulkan binary itself. The Mesa RADV driver already
  on the host drives the card through the same `/dev/dri` render node the ROCm
  device plugin injects. No custom image.

The ROCm route was built first and reached production quality: a Forgejo
repository (`takanao/lemonade-docker`) bakes in the `llamacpp-rocm` `b1302`
`gfx120X` build — a ~769 MB payload carrying its own ROCm 7 runtime — and the
chart pins `llamacpp.backend=system`. It worked: real ROCm/HIP offload on
gfx1200. Only then was it measured against Vulkan.

## Decision

**Serve from the upstream image on the `vulkan` backend, and keep the ROCm image
as a second, scaled-to-zero `lemonade-server-rocm` Deployment for
re-measurement.**

Backend selection is not expressible as a launch flag or environment variable in
lemonade — it lives in `config.json` on the recipe PVC — so each Deployment pins
its own backend with a small `jq` initContainer that merges the key idempotently
on every start. That is what keeps the choice declarative and in Git; see
[`k8s/lemonade-server/README.md`](../../k8s/lemonade-server/README.md) for the
resulting shape.

Three things decided it:

1. **Vulkan is measurably faster on this GPU** — roughly 20% single-request and
   47% at four concurrent requests, benchmarked between the two Deployments on
   the same PVCs and the same card. The gap matches what upstream reports for
   RDNA4 (e.g. [ggml-org/llama.cpp#20934](https://github.com/ggml-org/llama.cpp/issues/20934)),
   so it is a property of ROCm on this architecture today, not an artifact of
   the `system`-backend workaround.
2. **Vulkan carries no artifact of ours.** The ROCm route means a Forgejo
   repository, a pinned `llamacpp-rocm` build number, and a bundled ROCm 7
   userspace whose skew against the host amdgpu KMD has to be tracked. Vulkan's
   only dependency is the RADV driver already installed on the host — and
   unlike ComfyUI's ROCm wheels, nothing here is *inherent* to the workload.
   Paying that maintenance cost to be slower is the wrong trade.
3. **The maintenance is manual by construction.** Renovate cannot see the LAN
   registry (`renovate.json` disables it) or a Forgejo repository, so both the
   base tag and `LLAMACPP_ROCM_VERSION` are on human watch — the same gap
   ADR-0027 recorded for `comfyui-docker`. Acceptable for an artifact that runs
   only during a benchmark; not acceptable under a serving path.

Note that the image's *placement* was right even though the image lost: a
769 MB ROCm payload is exactly the "too large for a hosted runner" case ADR-0027
assigned to Forgejo on the self-hosted runner. The build path is not what is
being rejected here.

### Why keep the ROCm Deployment instead of deleting it

The decision rests on a measurement, and both sides of it move: lemonade may fix
the `gfx1200` detection (restoring the stock `rocm` backend with no custom image
at all), and ROCm's RDNA4 kernels are actively improving. Re-running the
comparison after such a change should be a `gpu-switch.sh` invocation, not a
rediscovery of the `system` backend's undocumented requirements and a fresh
image build.

The cost of keeping it is one chart template and a Deployment at
`replicaCount: 0`. It shares the `huggingface`/`llama`/`recipe` PVCs with the
primary Deployment, which is only safe because the single `amd.com/gpu`
guarantees the two never run at once (ADR-0027) — and because each side re-pins
its own backend at startup, a switch cannot leave the other's `config.json`
behind. It is labelled `homelab/gpu-switchable: "true"` and enumerated in the
`gpu-switch` RBAC values like any other GPU workload.

## Alternatives considered

- **ROCm via the `system` backend as the serving path** — the original plan, and
  it was implemented and running before being measured. Rejected on the numbers
  plus the permanent custom-image maintenance; *retained as the benchmark
  variant.*
- **Wait for the upstream fix** to `identify_rocm_arch_from_name` /
  `device_matches_constraint` and use the stock `rocm` backend — the cleanest
  end state, needing no image and no `system` backend. Rejected as a plan
  because the timeline is unbounded and, more decisively, because a working
  `rocm` backend would still be the slower one today. *Rejected as a plan,
  retained as a trigger to re-measure.*
- **Patch or fork lemonade** to expand the wildcard — a few lines against a
  lookup-table bug. Rejected: it trades a disposable benchmark image for a fork
  that must be rebased on every lemonade release, to reach a backend that loses
  the benchmark anyway. *Rejected.*
- **Fetch and unpack `llamacpp-rocm` onto the PVC from an initContainer**,
  avoiding a new repository and keeping everything inside this repo — genuinely
  attractive while ROCm looked like the answer, since lemonade already fetches
  backends at runtime by design. Rejected once Vulkan won; it would also have
  required expanding the `lemonade-llama` PVC and a 769 MB first-start download.
  *Rejected, never implemented.*

## Consequences

- **lemonade no longer exercises ROCm at all.** Only `comfyui` (PyTorch ROCm
  wheels) and `ollama` (bundled ROCm userspace) do. The host ROCm version from
  [`ansible/roles/rocm`](../../ansible/roles/rocm/) is therefore irrelevant to
  lemonade; what matters is the amdgpu kernel driver and the Mesa RADV
  userspace. One less consumer to check on a host ROCm upgrade.
- **A LAN image now exists that nothing serves from.** `lemonade-docker` must
  keep building for the benchmark variant to stay usable, and nothing will
  notice if it rots — Renovate cannot see it. Treat a failed re-measurement as
  "rebuild the image first", not as a result.
- **Two Deployments share three PVCs.** Safe today only because of GPU
  exclusivity. Anything that lets two GPU workloads run concurrently would break
  this pairing first.
- **Backend choice is a startup-time merge, not a manifest field.** Editing
  `config.json` by hand inside the pod is not durable — the initContainer
  overwrites the key on the next start. Change it in the chart.
- The README's earlier claim that RDNA4/gfx1200 is supported on both ROCm
  channels was wrong and has been corrected; it is recorded here because that
  claim is the kind of thing a future reader would otherwise re-derive by
  spending a day on it.
