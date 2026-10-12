# reloader

Deploys Stakater Reloader for annotated Secret/ConfigMap-driven restarts.

## Directory Structure

```
reloader/
├── Chart.yaml     # Wrapper chart with reloader as dependency
└── values.yaml    # watchGlobally: false
```

## Configuration

`reloader.reloader.watchGlobally: false` restricts the controller to its release
namespace, `reloader` ([upstream namespace scoping](https://docs.stakater.com/reloader/1.4/how-to-guides/namespace-scoping.html)).
It does not cover annotated workloads in other namespaces, including MCP,
slack-bot, and external-dns. Restart those Deployments explicitly after Secret
rotation until the controller scope is expanded.

Within the watched namespace, workloads opt in with:

```yaml
annotations:
  reloader.stakater.com/auto: "true"
```

or resource-specific annotations. Namespace scope and annotation opt-in are
separate controls.

## Environments

Deployed to the `prd` and `sandbox` clusters.
