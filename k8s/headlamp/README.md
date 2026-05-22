# Headlamp

Kubernetes Web UI deployed in the prd cluster.

## Initial Setup

Before ArgoCD syncs Headlamp, create a long-lived ServiceAccount token for login.

```bash
# Create ServiceAccount and ClusterRoleBinding
kubectl create serviceaccount headlamp-admin -n headlamp
kubectl create clusterrolebinding headlamp-admin \
  --clusterrole=cluster-admin \
  --serviceaccount=headlamp:headlamp-admin

# Create a non-expiring token Secret
kubectl apply -f - <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: headlamp-admin-token
  namespace: headlamp
  annotations:
    kubernetes.io/service-account.name: headlamp-admin
type: kubernetes.io/service-account-token
EOF

# Retrieve the token
kubectl get secret headlamp-admin-token -n headlamp \
  -o jsonpath='{.data.token}' | base64 -d
```

Store the token in OpenBao for future reference:

```bash
bao kv put secret/k8s/headlamp/admin-token value=<token>
```

To revoke access, delete the Secret and recreate it:

```bash
kubectl delete secret headlamp-admin-token -n headlamp
```

## Access

- prd: https://headlamp.prd.butaco.net

## Directory Structure

```
headlamp/
├── chart/
│   ├── Chart.yaml    # Wrapper chart with headlamp as dependency
│   └── values.yaml   # Common values (HTTPRoute gateway config)
└── values.yaml       # prd-specific values (hostname)
```
