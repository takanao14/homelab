# Headlamp

Read-only in-cluster Kubernetes UI for prd, using ServiceAccount
RBAC without cross-cluster kubeconfig Secrets.

## Directory Structure

```
headlamp/
├── chart/
│   ├── Chart.yaml        # Wrapper chart with headlamp as dependency
│   ├── values.yaml       # Common values (in-cluster mode, HTTPRoute, read-only RBAC)
│   └── templates/
│       ├── read-only-rbac.yaml  # Shared read-only permissions
│       ├── forward-auth.yaml    # External Authelia backend
│       ├── security-policy.yaml # HTTPRoute authentication gate
│       └── networkpolicy.yaml   # Gateway-only ingress
└── prd/values.yaml       # hostname: headlamp.prd.butaco.net
```

App of Apps enables Headlamp in prd only; sandbox has no inspection UI
(ADR-0070).

## Access

- prd: https://headlamp.prd.butaco.net

## Read-only access

Authelia admits `lab-platform-admins` through an Envoy Gateway `SecurityPolicy`
that fails closed. A NetworkPolicy restricts ingress to the shared Gateway pods
to prevent direct Service access from bypassing authentication (ADR-0074).
`-unsafe-use-service-account-token` removes the token prompt, so authenticated
visitors share the `headlamp` ServiceAccount, which is
read-only: the built-in `view` ClusterRole plus `headlamp-read-extra` for
cluster-scoped resources and the CRD groups in `readOnly.extraApiGroups`.

Admitted users can read what that role allows, including pod logs,
ConfigMaps, and pod specs, but not Secrets. Keep it that way:

- Never grant the core group wildcard or any group whose resources embed secret
  values (for example `helm.k0sproject.io` chart values).
- Never bind the ServiceAccount to a role with write verbs; that would give
  every admitted user those rights.
- Do not create a long-lived `headlamp-token` Secret.

Changes and inspection during Authelia outages use `kubectl` with the X.509
admin kubeconfig. Kubernetes authorization and audit records use the shared
ServiceAccount, not each user.

Authelia access rules are managed by `ansible/roles/authelia`. Apply its
playbook before syncing the Headlamp chart to avoid denying all users.

App of Apps restores Headlamp without kubeconfig Secrets
([ADR-0015](../../docs/adr/0015-headlamp-per-cluster-in-cluster-deployment.md)).
OpenBao kubeconfigs remain workstation-only.
