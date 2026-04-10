# docker

Custom Docker images for the homelab.

## Images

### argocd-helm-secrets-cmp

ArgoCD Config Management Plugin (CMP) container for rendering Helm charts with SOPS-encrypted secrets via [helm-secrets](https://github.com/jkroepke/helm-secrets).

Published to: `ghcr.io/takanao14/argocd-helm-secrets-cmp:latest`

**Base images:**

| Layer | Image | Purpose |
|-------|-------|---------|
| Base | `ghcr.io/helmfile/helmfile` | helmfile + helm + helm-secrets |
| Copied binary | `quay.io/argoproj/argocd` | `argocd-cmp-server` binary |

Runs as user `999` (non-root) as required by ArgoCD CMP.

**Used by:** ArgoCD `repoServer` as a sidecar container, configured in `k8s/argocd/values-common.yaml`.
