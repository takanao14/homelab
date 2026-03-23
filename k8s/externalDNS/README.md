# ExternalDNS for PowerDNS

Manages ExternalDNS to automatically register DNS records to PowerDNS from the homelab Kubernetes cluster. Uses Helmfile with dev/prd environments.

## Directory Structure

```
externalDNS/
├── helmfile.yaml              # Helmfile entrypoint
├── values-common.yaml.gotmpl  # Common values (references env vars)
├── values-dev.yaml.gotmpl     # Development environment values
├── values-prd.yaml.gotmpl     # Production environment values
├── secrets.enc.env            # SOPS-encrypted secrets (committed)
├── .envrc                     # Decrypts secrets (gitignored)
└── chart/                     # Helm chart
    ├── Chart.yaml
    ├── values.yaml
    └── templates/
        ├── deployment.yaml    # Includes checksum annotation for auto-restart on Secret change
        ├── rbac.yaml
        └── secret.yaml
```

## Prerequisites

- `helmfile` + `helm`
- PowerDNS server with HTTP API enabled
- Network connectivity from the Kubernetes cluster to the PowerDNS API

## Deployment

### 1. Set up secrets

```bash
cd k8s/externalDNS
sops edit secrets.enc.env
```

Then allow direnv:

```bash
direnv allow
```

### 2. Apply

```bash
# Development
helmfile -e dev apply

# Production
helmfile -e prd apply
```

## Configuration

| Value | Source | Description |
|-------|--------|-------------|
| `pdns.apiUrl` | hardcoded | PowerDNS API endpoint |
| `pdns.serverId` | hardcoded | PowerDNS server ID (`localhost`) |
| `pdns.apiKey` | `PDNS_API_KEY` env var | PowerDNS API key |
| `ownerId` | `values-<env>.yaml.gotmpl` | ID for records managed by this instance |
| `domainFilter` | `values-<env>.yaml.gotmpl` | Target domain filter |

The PowerDNS API key is stored in a Kubernetes Secret and passed to the container via `envFrom`. The Deployment has a checksum annotation on the Secret so it automatically restarts when the Secret changes.

## Secret Variables

| Variable | Description |
|----------|-------------|
| `PDNS_API_KEY` | PowerDNS HTTP API key |
