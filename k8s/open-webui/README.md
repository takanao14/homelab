# Open WebUI

Helm values for the upstream [open-webui chart](https://helm.openwebui.com/),
deployed via ArgoCD (rendered by `k8s/argocd/apps/templates/open-webui.yaml`,
enabled in `k8s/argocd/<env>/apps-values.yaml`). There is no local chart here;
the Application references the upstream chart and pulls these values through a
multi-source `$values` ref.

## Layout

- `values.yaml`: common values (Ollama/Lemonade/vLLM endpoints, persistence,
  route)
- `<env>/values.yaml`: per-env overrides (hostnames)

Currently deployed to `prd` only.

## Backend connections

The Ollama endpoint and the OpenAI-compatible Lemonade and vLLM endpoints are
declared in `values.yaml`. `ENABLE_PERSISTENT_CONFIG=false` makes these
environment values authoritative instead of connection settings saved through
the Admin UI. UI changes to ConfigVar-backed settings can affect the current
process but are discarded on restart.

Only one GPU backend is expected to run at a time through
`scripts/gpu-switch.sh`. Open WebUI remains running and discovers models from
the active endpoint; unavailable endpoints time out after three seconds.
Downloaded models remain managed manually in each backend's PVC.
