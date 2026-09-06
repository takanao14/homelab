# reloader

Deploys Stakater Reloader for annotated Secret/ConfigMap-driven restarts.

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

or resource-specific annotations, preventing cluster-wide unintended restarts.

## Environments

Deployed to the `prd` and `sandbox` clusters.
