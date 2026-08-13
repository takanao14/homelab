# Open WebUI

Values for the upstream [open-webui chart](https://helm.openwebui.com/), loaded
through Argo CD's multi-source `$values` reference.

## Layout

- `values.yaml`: common values (Ollama/Lemonade/vLLM endpoints, persistence,
  route)
- `<env>/values.yaml`: per-env overrides (hostnames)

Currently deployed to `prd` only.

## Backend connections

`values.yaml` declares Ollama, Lemonade, and vLLM endpoints.
`ENABLE_PERSISTENT_CONFIG=false` makes Git authoritative after restart.

GPU-switch runs one backend at a time. Open WebUI stays up, times out inactive
endpoints after three seconds, and leaves models in backend PVCs.
