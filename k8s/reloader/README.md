# reloader

Deploys one Stakater Reloader per cluster for explicitly opted-in workloads.

## Directory Structure

```text
reloader/
├── Chart.yaml
├── values.yaml          # Shared scoped watch and annotation reload strategy
├── prd/values.yaml      # external-dns, mcp, slack-bot
└── sandbox/values.yaml  # reloader-check
```

## Configuration

`reloader.reloader.watchGlobally: false` and environment-specific `namespaces`
create Roles and RoleBindings only in the allowed namespaces. The release
namespace, `reloader`, is automatically included. Reloader syncs at wave 2,
after consumer Applications create their namespaces at wave 1.

`autoReloadAll: false` requires opt-in. Consumers use
`secret.reloader.stakater.com/auto: "true"` on Deployment metadata for Secret
changes only. Allowed namespaces grant access to all their Secrets; annotations
restrict restart behavior, not RBAC permissions.

`reloadStrategy: annotations` updates the Pod-template restart marker. Consumer
Applications ignore only that marker and use `RespectIgnoreDifferences=true`
to preserve it during syncs. ESO refresh timing still controls when a rotated
Secret reaches the workload; MCP and external-dns use `Recreate` and briefly
interrupt service during restarts.

## Environments

prd watches external-dns, MCP, and slack-bot. sandbox watches the disposable
[reloader-check](../reloader-check/README.md) consumers.
