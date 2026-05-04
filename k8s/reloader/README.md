# reloader

Deploys [Stakater Reloader](https://github.com/stakater/Reloader) to automatically trigger rolling restarts of Deployments when their referenced Secrets or ConfigMaps change. Managed by ArgoCD.

## Directory Structure

```
reloader/
├── Chart.yaml     # Wrapper chart with reloader as dependency
└── values.yaml    # watchGlobally: false
```

## Configuration

`watchGlobally` is set to `false`, so Reloader only watches resources annotated with:

```yaml
annotations:
  reloader.stakater.com/auto: "true"
```

or specific secret/configmap annotations. This prevents unintended restarts across all namespaces.

## Environments

Deployed to both `dev` and `prd` clusters.

## Usage

```yaml
# In ArgoCD Application (k8s/argocd/{env}/apps/reloader.yaml)
source:
  path: k8s/reloader
  helm:
    releaseName: reloader
destination:
  namespace: reloader
```
