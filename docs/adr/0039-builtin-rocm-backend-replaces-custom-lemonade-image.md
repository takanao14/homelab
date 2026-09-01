# ADR-0039: Lemonade v11 serves gfx1200 through the built-in ROCm backend, retiring the custom image and the second Deployment

- **Status:** Accepted
- **Date:** 2026-09-01
- **Related:** [ADR-0028](0028-lemonade-vulkan-backend-over-rocm-on-rdna4.md) and
  [ADR-0029](0029-rocm-serving-path-for-mxfp4-models.md) (both superseded by
  this decision), [ADR-0027](0027-gpu-workload-switching-web-ui.md),
  [ADR-0041](0041-opencode-connects-to-ollama.md),
  [`k8s/lemonade-server/README.md`](../../k8s/lemonade-server/README.md)

## Context

ADR-0028 chose Vulkan because Lemonade v10.8.0 rejected the RX 9060 XT: its
family matcher compared the KFD ISA name `gfx1200` against the wildcard token
`gfx120X` with exact string equality. ADR-0029 then built a custom image
(`lemonade-docker`, base v10.8.0 plus a pinned `llamacpp-rocm` build) driven
through `llamacpp.backend=system`, because the `system` backend has no
GPU-family gate. Both records exist to work around that one bug.

The bug is fixed upstream. Verified against v11.8.1 on 2026-09-01:

- `device_matches_constraint` in `src/cpp/server/system_info.cpp` changed from
  `allowed_families.count(device_family) > 0` to a matcher documented as
  "a trailing 'X' in an allowed family acts as a wildcard".
- A throwaway probe Pod on the RX 9060 XT reported
  `llamacpp rocm installable` where v10.8.0 reports
  `Unsupported GPU: gfx1200`. The remaining `Unsupported GPU: gfx1200` rows
  (`ds4`, `thenoise`) are Strix-only backends and correct.
- `llamacpp_server.cpp` resolves the ROCm `nightly` channel to
  `lemonade-sdk/llamacpp-rocm` — the same release assets the custom image bakes
  in. MXFP4 therefore runs on identical binaries either way.
- `llamacpp.rocm_bin` accepts a specific release tag per channel, so the
  built-in path can be pinned by build number rather than tracking `latest`.
- The backend is a **per-model** property in v11: `llamacpp_backend` is a
  `recipe_options` field on the model, not a server-wide setting. One server
  process can hold a `rocm` model and a `vulkan` model at once.

Upstream issue [#2319](https://github.com/lemonade-sdk/lemonade/issues/2319)
remains open, but its live comments describe a different symptom (an iGPU
outranking a dGPU during selection) that does not apply to this single-dGPU node.

## Decision

**Serve ROCm through the upstream image's built-in `llamacpp:rocm` backend and
retire the custom image.** The custom image and the `lemonade-docker`
repository exist only to bypass a bug that no longer exists.

**Collapse to one Deployment.** The two Deployments only ever existed because
ADR-0029 needed a second image; the backend split was never the reason. With
`llamacpp_backend` selectable per model, a single server serves MXFP4 on `rocm`
and anything else on `vulkan` simultaneously, which is strictly more capable
than switching between two Deployments that cannot run at the same time anyway
(ADR-0027 keeps the GPU exclusive).

This retires ADR-0028's per-Deployment `config.json` pin. That mechanism existed
so two Deployments sharing one recipe PVC could disagree about the backend;
with one Deployment there is nothing to disagree. Server-wide ROCm settings
(`rocm_channel=nightly`, `llamacpp.rocm_bin` pinned to a build number) still
belong in `config.json`. The chart renders a sparse file from Helm values and an
initContainer copies it from a ConfigMap to a writable per-Pod config volume;
the v11 image does not contain `jq`.

ADR-0029's finding stands and is better served by this shape: backend choice
depends on model, quantization, and concurrency, so it belongs on the model,
not on a Deployment.

## Validation

Accepted after the production rollout on 2026-09-01:

- Argo CD synced the single `lemonade-server` Deployment at v11.8.1 and pruned
  the custom ROCm Deployment, Service, HTTPRoute, and the empty
  `lemonade-llama` PVC.
- The effective configuration reported `rocm_channel=nightly` and
  `llamacpp.rocm_bin=b1319`; the built-in installer persisted that backend in
  the recipe PVC.
- Qwen3-0.6B Q4_0 loaded with `llamacpp_backend=rocm`, reported `gpu` / `ready`,
  and completed an OpenAI-compatible request through the service.
- Prometheus `amd_gpu_used_vram` rose from 65 MiB to 901 MiB. The running child
  process came from `bin/llamacpp/rocm-nightly`, and its `version.txt` was
  `b1319`.
- The Service, external HTTPRoute, and Argo CD Application were healthy. Both
  persistent caches were writable by the v11 process running as uid 10001.
- The `takanao/lemonade-docker` Forgejo repository was archived read-only after
  the rollout. Registry tags `b1302` and `b1319` remain available only as cold
  rollback artifacts and are not referenced by active configuration.

## Migration constraints

The base image bump is not a tag change. Verified on the v11.8.1 probe:

- **The container no longer runs as root.** `uid=10001(lemonade) gid=999`,
  `HOME=/opt/lemonade`. The chart mounts caches at `/root/.cache/huggingface`
  and `/root/.cache/lemonade`; those paths are wrong under v11.8.1, and the
  root-owned `openebs-hostpath` PVCs are not writable by uid 10001 without an
  `fsGroup`.
- **`/opt/lemonade/llama` collides.** v11.8.1 ships `lemonade`, `lemond`,
  `llama`, and `resources` under `/opt/lemonade`; mounting an empty PVC over
  `llama` hides shipped content.
- **The CLI split.** `lemonade` is now an HTTP client and the server is a
  separate `lemond` binary; `lemonade serve` no longer exists.
- **`rocm_requires_cwsr_fix` does not affect this GPU.** Despite the generic
  descriptor flag, v11.8.1 uses it only to reject `gfx1151` (Strix Halo) when
  required kernel sysfs properties are absent. It does not alter the ROCm
  process environment and does not apply to this node's `gfx1200` GPU.

## Consequences

- **The `lemonade-docker` repository is retired**, along with the manual
  monitoring burden ADR-0029 accepted for its un-Renovatable base tag. Pinning
  moves to `llamacpp.rocm_bin` in values, which Renovate still cannot see — the
  burden shrinks but does not disappear.
- **ADR-0029's measurements no longer describe anything deployable.** They were
  taken on Lemonade 10.8.0 with `b1302` (ROCm 7.15) through the `system`
  backend. Re-measurement must wait until this migration lands, or it measures
  a configuration that is about to be deleted.
- `src/cpp/resources/benchmark_forks.json` defines benchmark forks including
  `llamacpp-rocm-nightly`, so the re-measurement may be driveable by Lemonade
  itself rather than by hand-written request loops. Comparing backends inside
  one process also removes image and lemonade-version skew as variables.
- **The chart loses its `-rocm` templates** (`deployment-rocm.yaml`,
  `service-rocm.yaml`, `httproute-rocm.yaml`) and the `rocm.*` values block,
  and gains an `fsGroup` and corrected mount paths.
- **`lemonade-rocm.prd.butaco.net` disappears**, and no client configuration in
  this repository depends on it: [ADR-0041](0041-opencode-connects-to-ollama.md)
  already moved `opencode.json` to Ollama and removed the `lemonade-rocm`
  provider. ADR-0029's claim that the ROCm Deployment sits on a configured
  serving path no longer holds, which is what makes this migration low-risk.
- gpu-switch (ADR-0027) loses one target, leaving one lemonade workload.

## Alternatives considered

- **Stay on v10.8.0 with the custom image** — freezes the deployment on a
  release whose central defect is fixed upstream, and keeps a private image on
  the serving path for no remaining reason.
- **Bump only the custom image's base to v11.8.1** — retains the `system`
  backend hack after its justification disappeared. It also still requires
  solving every migration constraint above, so it buys nothing.
- **Use the ROCm `stable` channel instead of `nightly`** — stable resolves to
  `lemonade-sdk/llama.cpp`, a different repository with different build tags,
  and would change the binaries currently serving MXFP4. Worth measuring later;
  not a safe move to make in the same step as the migration.
- **Drop ROCm entirely and serve everything on Vulkan** — ADR-0029 measured
  ROCm 21.1% faster on four-request aggregate throughput, so discarding the
  backend would discard a real result. Collapsing the Deployments keeps both
  backends available at no structural cost, so there is nothing to gain by
  dropping one.
- **Keep two Deployments after unifying the image** — they would differ only by
  a config value, duplicating a Deployment, Service, HTTPRoute, and hostname to
  express a choice that now lives on the model. It also preserves the shared-PVC
  contention that forced ADR-0028's per-Deployment pin in the first place.
