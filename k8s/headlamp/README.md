# Headlamp

Kubernetes Web UI, deployed in-cluster per environment (prd, dev). Each
cluster runs its own Headlamp instance authenticated via ServiceAccount RBAC —
no cross-cluster kubeconfig secrets. See the design change note below.

## Directory Structure

```
headlamp/
├── chart/
│   ├── Chart.yaml    # Wrapper chart with headlamp as dependency
│   └── values.yaml   # Common values (in-cluster mode, HTTPRoute gateway config)
├── prd/values.yaml   # hostname: headlamp.prd.butaco.net
└── dev/values.yaml   # hostname: headlamp.dev.butaco.net
```

Deployed via the app-of-apps chart (`k8s/argocd/apps/templates/headlamp.yaml`);
enable per environment in `k8s/argocd/<env>/apps-values.yaml`.

## Access

- prd: https://headlamp.prd.butaco.net
- dev: https://headlamp.dev.butaco.net

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

## Design Note: per-cluster in-cluster instead of central multi-cluster

Headlamp originally ran only in prd with `-in-cluster=false`, mounting
ESO-synced kubeconfigs (OpenBao `kubeconfig/dev`, `kubeconfig/prd`) to show
both clusters in one UI. That was replaced by per-cluster in-cluster
deployments because:

- static kubeconfigs mounted in prd spanned cluster boundaries (blast radius)
  and went stale on every k0s cluster rebuild;
- in-cluster mode needs no secret material at all, so a rebuilt cluster gets a
  working Headlamp from the app-of-apps bootstrap with zero manual steps.

The OpenBao `kubeconfig/*` entries remain in use for workstation kubeconfig
sync (`scripts/secrets/get-kubeconfig.sh`); Headlamp no longer reads them.
