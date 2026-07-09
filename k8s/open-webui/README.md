# Open WebUI

Helm values for the upstream [open-webui chart](https://helm.openwebui.com/),
deployed via ArgoCD (rendered by `k8s/argocd/apps/templates/open-webui.yaml`,
enabled in `k8s/argocd/<env>/apps-values.yaml`). There is no local chart here;
the Application references the upstream chart and pulls these values through a
multi-source `$values` ref.

## Layout

- `values.yaml`: common values (Ollama/Lemonade endpoints, persistence, route)
- `<env>/values.yaml`: per-env overrides (hostnames)

Currently deployed to `prd` only.
