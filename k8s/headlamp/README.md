# Headlamp

Kubernetes Web UI, deployed in-cluster for prd and sandbox. Each
cluster runs its own Headlamp instance authenticated via ServiceAccount RBAC —
no cross-cluster kubeconfig secrets. See the design change note below.

## Directory Structure

```
headlamp/
├── chart/
│   ├── Chart.yaml        # Wrapper chart with headlamp as dependency
│   └── values.yaml       # Common values (in-cluster mode, HTTPRoute gateway config)
├── prd/values.yaml       # hostname: headlamp.prd.butaco.net, https listener
└── sandbox/values.yaml   # hostname: headlamp.sandbox.butaco.net, http listener (ADR-0010)
```

Deployed via the app-of-apps chart (`k8s/argocd/apps/templates/headlamp.yaml`);
enable per environment in `k8s/argocd/<env>/apps-values.yaml`.

## Access

- prd: https://headlamp.prd.butaco.net
- sandbox: http://headlamp.sandbox.butaco.net

## Login Token (per cluster)

The Helm chart creates a ServiceAccount `headlamp` with a `cluster-admin`
binding. After ArgoCD syncs, create a long-lived token Secret for login on
each cluster:

```bash
kubectl apply -f - <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: headlamp-token
  namespace: headlamp
  annotations:
    kubernetes.io/service-account.name: headlamp
type: kubernetes.io/service-account-token
EOF

# Retrieve the token
kubectl get secret headlamp-token -n headlamp \
  -o jsonpath='{.data.token}' | base64 -d
```

To revoke access, delete and recreate the Secret:

```bash
kubectl delete secret headlamp-token -n headlamp
```

## Design Note

Headlamp originally ran only in prd, mounting ESO-synced kubeconfigs to show
multiple clusters in one UI. It now runs in-cluster per cluster — no kubeconfig
secrets, so a rebuilt cluster gets a working Headlamp from the app-of-apps
bootstrap alone. See
[ADR-0015](../../docs/adr/0015-headlamp-per-cluster-in-cluster-deployment.md)
for the rationale and rejected alternatives (including the addendum on the
sandbox deployment). The OpenBao `kubeconfig/*` entries remain in use for
workstation kubeconfig sync (`scripts/secrets/get-kubeconfig.sh`); Headlamp no
longer reads them.
