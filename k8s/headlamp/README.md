# Headlamp

Read-only in-cluster Kubernetes UI for each environment, using ServiceAccount
RBAC without cross-cluster kubeconfig Secrets.

## Directory Structure

```
headlamp/
├── chart/
│   ├── Chart.yaml        # Wrapper chart with headlamp as dependency
│   ├── values.yaml       # Common values (in-cluster mode, HTTPRoute, read-only RBAC)
│   └── templates/
│       └── read-only-rbac.yaml  # ServiceAccount bindings to view + extra read groups
├── prd/values.yaml       # hostname: headlamp.prd.butaco.net, https listener
└── sandbox/values.yaml   # hostname: headlamp.sandbox.butaco.net, http listener (ADR-0010)
```

App of Apps enables Headlamp per environment.

## Access

- prd: https://headlamp.prd.butaco.net
- sandbox: http://headlamp.sandbox.butaco.net

## Read-only access

Headlamp has no authentication. `-unsafe-use-service-account-token` removes the
token prompt, so every visitor uses the `headlamp` ServiceAccount, which is
read-only: the built-in `view` ClusterRole plus `headlamp-read-extra` for
cluster-scoped resources and the CRD groups in `readOnly.extraApiGroups`.

Anyone on the LAN can therefore read what that role allows, including pod logs,
ConfigMaps, and pod specs, but not Secrets. Keep it that way:

- Never grant the core group wildcard or any group whose resources embed secret
  values (for example `helm.k0sproject.io` chart values).
- Never bind the ServiceAccount to a role with write verbs; that would give
  every LAN client those rights.
- Do not create a long-lived `headlamp-token` Secret.

Changes go through `kubectl` with the X.509 admin kubeconfig. Read-only access
does not depend on any identity service.

## Design Note

Headlamp moved from a prd multi-cluster kubeconfig design to per-cluster
instances, allowing App of Apps alone to restore it. See
[ADR-0015](../../docs/adr/0015-headlamp-per-cluster-in-cluster-deployment.md)
for the rationale. OpenBao kubeconfigs remain workstation-only.
