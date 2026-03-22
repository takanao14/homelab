# Forgejo

A self-hosted Git service (Forgejo) deployed on the Kubernetes cluster within the homelab.

## Architecture

- **Chart**: `forgejo/forgejo` (OCI: `code.forgejo.org/forgejo-helm`)
- **Deployment**: Managed using `helmfile`

## Deployment

Set the following environment variables before running `helmfile`.
All variables use `envRequired` in the template — missing any will cause an immediate rendering error, preventing a broken deploy.

```bash
export FORGEJO_DOMAIN=git.exmaple.com
export FORGEJO_SSH_DOMAIN=gitssh.exmaple.com
export FORGEJO_ADMIN_USERNAME=admin
export FORGEJO_ADMIN_PASSWORD=your_secure_password
export FORGEJO_ADMIN_EMAIL=admin@example.com

helmfile apply
```

A `.envrc.sample` is provided as a commit-safe template. Copy it and fill in the actual values:

```bash
cp .envrc.sample .envrc
# Edit .envrc with actual values
direnv allow
```

You can also manage these values with `.envrc` (via `direnv`):

| Variable | Description | Example |
| :--- | :--- | :--- |
| `FORGEJO_DOMAIN` | HTTP domain for Forgejo UI | `git.exmaple.com` |
| `FORGEJO_SSH_DOMAIN` | SSH domain for Forgejo | `gitssh.exmaple.com` |
| `FORGEJO_ADMIN_USERNAME` | Initial admin username | `admin` |
| `FORGEJO_ADMIN_PASSWORD` | Initial admin password | `your_secure_password` |
| `FORGEJO_ADMIN_EMAIL` | Initial admin email | `admin@example.com` |

## Configuration Details

- **Domain / SSH Domain**: Configured via `FORGEJO_DOMAIN` / `FORGEJO_SSH_DOMAIN` environment variables (see `.envrc`).
- **Persistence**: 100Gi Persistent Volume
- **Service**:
    - HTTP (Port 80): via `LoadBalancer`.
    - SSH (Port 22): via `LoadBalancer`.
    - Hostname is automatically resolved by `external-dns` using the same domain variables.

## Admin Configuration

Admin credentials are injected via environment variables.
See `values.yaml.gotmpl` for details.
