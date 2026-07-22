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

Deployed to the `prd` and `sandbox` clusters.

## Usage

The `reloader` Application is rendered by the app-of-apps chart
(`k8s/argocd/apps`) and enabled per environment in
`k8s/argocd/<env>/apps-values.yaml`. The generated Application:

```yaml
source:
  path: k8s/reloader
  helm:
    releaseName: reloader
destination:
  namespace: reloader
```
