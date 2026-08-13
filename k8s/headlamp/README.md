# Headlamp

In-cluster Kubernetes UI for each environment, authenticated by ServiceAccount
RBAC without cross-cluster kubeconfig Secrets.

## Directory Structure

```
headlamp/
├── chart/
│   ├── Chart.yaml        # Wrapper chart with headlamp as dependency
│   └── values.yaml       # Common values (in-cluster mode, HTTPRoute gateway config)
├── prd/values.yaml       # hostname: headlamp.prd.butaco.net, https listener
└── sandbox/values.yaml   # hostname: headlamp.sandbox.butaco.net, http listener (ADR-0010)
```

App of Apps enables Headlamp per environment.

## Access

- prd: https://headlamp.prd.butaco.net
- sandbox: http://headlamp.sandbox.butaco.net

## Login Token (per cluster)

The chart binds `headlamp` to `cluster-admin`. After sync, create its login token:

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

Headlamp moved from a prd multi-cluster kubeconfig design to per-cluster
instances, allowing App of Apps alone to restore it. See
[ADR-0015](../../docs/adr/0015-headlamp-per-cluster-in-cluster-deployment.md)
for the rationale. OpenBao kubeconfigs remain workstation-only.
