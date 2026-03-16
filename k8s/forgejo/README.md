# Forgejo

A self-hosted Git service (Forgejo) deployed on the Kubernetes cluster within the homelab.

## Architecture

- **Chart**: `forgejo/forgejo` (OCI: `code.forgejo.org/forgejo-helm`)
- **Deployment**: Managed using `helmfile`

## Deployment

Set the following environment variables before running `helmfile`:

```bash
export FORGEJO_ADMIN_USERNAME=admin
export FORGEJO_ADMIN_PASSWORD=your_secure_password
export FORGEJO_ADMIN_EMAIL=admin@example.com

helmfile apply
```

## Configuration Details

- **Domain**: `git.k8s.homelab.internal`
- **SSH Domain**: `gitssh.k8s.homelab.internal`
- **Persistence**: 100Gi Persistent Volume
- **Service**:
    - HTTP (Port 80): via `LoadBalancer`.
    - SSH (Port 22): via `LoadBalancer`.
    - Hostname is automatically resolved by `external-dns`.

## Admin Configuration

Admin credentials are injected via environment variables.
See `values.yaml.gotmpl` for details.
