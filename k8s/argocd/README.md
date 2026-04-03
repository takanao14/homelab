# ArgoCD

ArgoCD configuration for Kubernetes cluster management via GitOps. Supports two environments: `dev` and `prd`.

## Directory Structure

```
argocd/
├── values-common.yaml    # Common configuration for all environments
├── dev/
│   ├── helmfile.yaml     # Deployment config for dev environment
│   ├── values.yaml       # Environment-specific values for dev
│   └── apps/
│       ├── argocd.yaml       # ArgoCD self-managed Application
│       ├── external-dns.yaml # External DNS Application
│       └── meshcentral.yaml  # MeshCentral Application
└── prd/
    ├── helmfile.yaml     # Deployment config for prd environment
    ├── values.yaml       # Environment-specific values for prd
    └── apps/
        ├── argocd.yaml       # ArgoCD self-managed Application
        └── external-dns.yaml # External DNS Application
```

## Environments

| Environment | Cluster | ArgoCD URL |
|-------------|---------|------------|
| dev         | dev-homelab | `argocd.dev.butaco.net` |
| prd         | prd-homelab | `argocd.prd.butaco.net` |

## Initial Deployment

ArgoCD is initially deployed using helmfile, and subsequently self-manages itself.

```bash
# prd environment
cd k8s/argocd/prd
helmfile apply

# dev environment
cd k8s/argocd/dev
helmfile apply
```

A helmfile hook will interrupt the deployment if the context of the target cluster is incorrect.

## helm-secrets Plugin (CMP)

A Config Management Plugin (CMP) defined in `values-common.yaml` allows SOPS-encrypted secrets to be managed in Git.

- Encryption method: Age
- Custom image: `ghcr.io/takanao14/argocd-helm-secrets-cmp:latest`
- Private key is managed via a Kubernetes secret (`helm-secrets-private-keys`)

### Private Key Placement

Before deploying ArgoCD, register the Age private key as a Kubernetes secret.

```bash
kubectl create secret generic helm-secrets-private-keys \
  --from-file=key.txt=/path/to/age-private-key.txt \
  -n argocd
```

The CMP container references `/helm-secrets-private-keys/key.txt` as the Age private key (specified by the `SOPS_AGE_KEY_FILE` environment variable).

## Apps

ArgoCD Applications reference the Git repository as a source, with automated sync enabled (including pruning and self-healing).

| Application | namespace | Environment | secrets |
|-------------|-----------|-------------|---------|
| argocd | argocd | dev, prd | none |
| external-dns | dns-homelab | dev, prd | yes (helm-secrets) |
| meshcentral | meshcentral | dev only | none |
