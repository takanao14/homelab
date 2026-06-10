# Headlamp

Kubernetes Web UI deployed in the prd cluster.

## Initial Setup

The Helm chart automatically creates a ServiceAccount `headlamp` with `cluster-admin` binding.
After ArgoCD syncs, create a long-lived token Secret for login.

```bash
# Create a non-expiring token Secret for the headlamp ServiceAccount
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

Store the token in OpenBao for future reference:

```bash
bao kv put secret/k8s/headlamp/admin-token value=<token>
```

To revoke access, delete the Secret and recreate it:

```bash
kubectl delete secret headlamp-token -n headlamp
```

## Access

- prd: https://headlamp.prd.butaco.net

## Adding Dev Cluster

Headlamp supports multi-cluster by mounting a kubeconfig file as a volume.
The dev kubeconfig is stored in OpenBao at `secret/kubeconfig/dev` and synced via ESO.

### 1. Store kubeconfigs in OpenBao

```bash
./scripts/set-kubeconfig.sh
```

This stores `~/.kube/dev.yaml` and `~/.kube/prd.yaml` in OpenBao.

### 2. Sync and verify

Push the changes and let ArgoCD sync. Headlamp will mount the dev kubeconfig at
`/headlamp/kubeconfig` and expose it as a selectable cluster in the UI.

## Directory Structure

```
headlamp/
├── chart/
│   ├── Chart.yaml    # Wrapper chart with headlamp as dependency
│   └── values.yaml   # Common values (HTTPRoute gateway config)
└── values.yaml       # prd-specific values (hostname)
```
