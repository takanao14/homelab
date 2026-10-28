# strata

[Strata](https://github.com/Niko1221/Strata) serves Qwen3.8-Flash-Next IQ3_XXS
on the prd AMD GPU worker, keeping part of the experts in RAM
([ADR-0080](../../docs/adr/0080-strata-as-a-gpu-switch-workload.md)). The chart
runs the upstream `rocm-gfx120x` image pinned by digest in
[chart/values.yaml](chart/values.yaml).

## Access

| Client | Address |
|--------|---------|
| External | `https://strata.prd.butaco.net` |
| In-cluster | `http://strata.strata.svc.cluster.local:8080` |

Every request except `GET /health` needs `Authorization: Bearer <key>`. Use
`/v1` as the OpenAI-compatible base URL and the root as the Anthropic-compatible
base URL; read model names from `GET /v1/models`.

## Secrets

The API key comes from OpenBao `secret/k8s/strata/api` (`api-key`, at least 16
characters) through ESO. Add it to the encrypted `openbao_secrets` inventory and
seed it with `playbooks/ops/openbao/seed_secrets.yaml`. Reading it requires the
`k8s-strata` policy on the prd Kubernetes auth role, applied by
`playbooks/ops/openbao/configure.yaml`. Reloader restarts the Deployment after
the Secret changes.

## GPU Switching

Git keeps the Deployment at zero replicas. Activate it with gpu-switch:

```bash
scripts/gpu-switch.sh strata
```

The first start downloads about 76 GB, then writes a 40 GB pack and a 6 GB draft
layer to `strata-data`; the startup probe allows four hours. Later starts load
from the PVC. Follow progress with:

```bash
kubectl -n strata logs -f deploy/strata
kubectl -n strata exec deploy/strata -- strata monitor --once
```

## Runtime Constraints

- Do not add a memory limit. Strata sizes its placement from the node's RAM.
- `FAMILY`, `MODEL` and `VISION` are saved as a preset in `/data`. Changing the
  model downloads it again; delete the old files under `/data/Strata-data`
  to reclaim space.
- The Pod inherits an 8 MiB `memlock` limit. If the logs report a pinning
  failure, raise the limit for the k0s worker on gpuvm1 (ADR-0080).
