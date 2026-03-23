# Forgejo

Self-hosted Git service deployed on the homelab Kubernetes cluster.

## Architecture

- **Chart**: `forgejo/forgejo` (OCI: `code.forgejo.org/forgejo-helm`)
- **Deployment**: Managed with `helmfile`
- **Persistence**: 100Gi Persistent Volume
- **Service**: HTTP (port 80) and SSH (port 22) via LoadBalancer; hostnames registered automatically by ExternalDNS

## Directory Structure

```
forgejo/
├── helmfile.yaml
├── values.yaml.gotmpl       # All values; domain/SSH domain as local Go template vars
├── secrets.enc.env          # SOPS-encrypted secrets (committed)
└── .envrc                   # Decrypts secrets (gitignored)
```

## Deployment

### 1. Set up secrets

```bash
cd k8s/forgejo
sops edit secrets.enc.env
direnv allow
```

### 2. Apply

```bash
helmfile apply
```

## Configuration

Domains are defined as Go template variables at the top of `values.yaml.gotmpl` (not environment variables, since they are not sensitive):

```gotmpl
{{- $domain    := "git.prd.butaco.net" -}}
{{- $sshDomain := "gitssh.prd.butaco.net" -}}
```

Admin credentials are injected via `requiredEnv` — a missing variable causes an immediate rendering error, preventing a broken deploy.

## Secret Variables

| Variable | Description |
|----------|-------------|
| `FORGEJO_ADMIN_USERNAME` | Initial admin username |
| `FORGEJO_ADMIN_PASSWORD` | Initial admin password |
| `FORGEJO_ADMIN_EMAIL` | Initial admin email |
