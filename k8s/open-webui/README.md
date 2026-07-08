# Open WebUI

Helm values for the upstream [open-webui chart](https://helm.openwebui.com/),
deployed via ArgoCD (`k8s/argocd/<env>/apps/open-webui.yaml`). There is no
local chart here; the Application references the upstream chart and pulls
these values through a multi-source `$values` ref.

## Layout

- `values.yaml`: common values (Ollama/Lemonade endpoints, persistence, route)
- `<env>/values.yaml`: per-env overrides (hostnames)

Currently deployed to `dev` only.
