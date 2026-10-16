# ADR-0071: Scope one Reloader per cluster to explicit namespaces

- **Status:** Accepted
- **Accepted:** 2026-10-15
- **Category:** Kubernetes platform
- **Date:** 2026-10-12
- **Review trigger:** Before implementing automatic Secret-driven restarts
- **Related:** [ADR-0026](0026-eso-over-helm-secrets-for-in-cluster-secrets.md), [ADR-0070](0070-no-staging-sandbox-validates-the-cluster-platform.md)

## Context

The prd Reloader watches only its release namespace, `reloader`. Runtime logs
and RBAC confirm this scope; annotated consumers in other namespaces receive
no restart after ESO updates their Secrets. external-dns, Grafana, and SNMP
exporter also put their opt-in annotations on Pod templates rather than the
Deployment metadata Reloader reads. MCP and slack-bot use Deployment metadata.

The pinned Reloader chart 2.2.18 supports an explicit namespace list with
namespace-scoped Roles and RoleBindings. One controller can therefore serve
the consumers without cluster-wide Secret access or multiple installations.

## Decision

Keep one Reloader per cluster with `watchGlobally: false` and explicit
`namespaces`. Set prd to `external-dns`, `mcp`, and `slack-bot`;
set sandbox to `reloader-check`, a namespace holding only disposable consumers,
so every sandbox bootstrap exercises the cross-namespace RBAC. Supply its
Secret only temporarily when testing rollouts. The chart also includes the
release namespace.
Put shared behavior in `k8s/reloader/values.yaml` and the lists in environment
value files referenced by the Reloader Application. Target namespaces remain
owned by their workload Applications; account for their existence before
Reloader creates cross-namespace RBAC during bootstrap.

Keep `autoReloadAll: false`. Opt consumers into Secret changes using
`secret.reloader.stakater.com/auto` on Deployment metadata. MCP, slack-bot,
and external-dns need this behavior. Add ConfigMap-triggered restarts only
where an application cannot reload its configuration itself.

Exclude SNMP exporter. The upstream chart (9.18.1) cannot annotate Deployment
metadata, and its community string rarely changes; restart it manually after
rotation and remove its ineffective Pod-template annotation.

Do not opt Grafana into restarts solely for `grafana-admin` rotation. The prd
Grafana's persistent database retains the existing admin password; the initial
password setting is applied only on first run. Password rotation must update
the stored account separately. Sandbox Grafana is ephemeral and reads only a
sandbox Secret (ADR-0070), but has nothing else that needs a restart. Remove
the ineffective Pod-template annotations from both Grafana value files.

Use `reloadStrategy: annotations`. Target Applications ignore only
`/spec/template/metadata/annotations/reloader.stakater.com~1last-reloaded-from`
on the opted-in Deployments and set `RespectIgnoreDifferences=true`. Share
the rule through chart helpers where applicable; do not ignore all annotations,
environment variables, or Pod templates. Argo CD must preserve the restart
marker during unrelated syncs without suppressing other drift.

## Alternatives considered

- **Cluster-wide watch:** simplest configuration, but grants Secret access and
  workload mutation beyond the required namespaces.
- **Namespace label selector:** convenient discovery, but uses cluster-wide
  RBAC; labels filter behavior rather than authorization.
- **One Reloader per namespace:** isolates controller identities, but duplicates
  releases and runtime overhead for a single-operator homelab.
- **Restart from rotation scripts:** misses changes made through other update
  paths and couples each writer to ESO synchronization and workload lifecycle.

## Consequences

Adding a consumer namespace requires updating that environment's allowlist.
Within an allowed namespace, Reloader can read all Secrets and update supported
workloads; opt-in annotations restrict behavior, not its RBAC privileges.

Restart timing follows ESO synchronization, currently hourly for these Secrets.
MCP and external-dns use `Recreate`, so reloads interrupt service briefly.
Caller-token rotation still requires coordinating MCP clients and servers;
Reloader does not update VM clients or sequence token retirement.

Acceptance requires a changed Secret to trigger one rollout, unchanged data to
trigger none, excluded namespaces and unannotated consumers to stay unaffected,
and an unrelated Argo CD sync to preserve the restart marker. RBAC must deny
Secret access outside the allowlist, and a fresh bootstrap must reconcile the
cross-namespace bindings. Use test values, never production token changes, to
exercise the mechanism.

References: [pinned Reloader settings](https://github.com/stakater/Reloader/blob/v1.4.22/deployments/kubernetes/chart/reloader/values.yaml),
[Reloader with Argo CD](https://docs.stakater.com/reloader/1.4/how-to-guides/use-reloader-with-argocd.html),
[Argo CD sync options](https://argo-cd.readthedocs.io/en/stable/user-guide/sync-options/#respect-ignore-differences-configs),
[Grafana initial admin password](https://grafana.com/docs/grafana/latest/setup-grafana/configure-grafana/#admin_password).
