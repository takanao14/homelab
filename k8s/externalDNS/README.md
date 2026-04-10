# ExternalDNS

Automatically registers DNS records in PowerDNS from Kubernetes HTTPRoute resources. Managed by ArgoCD with the helm-secrets plugin.

## Directory Structure

```
externalDNS/
├── values-common.yaml         # PowerDNS API endpoint (shared)
├── dev/
│   └── values.yaml            # domainFilter: dev.butaco.net.
├── prd/
│   └── values.yaml            # domainFilter: prd.butaco.net.
└── chart/                     # Local Helm chart
    ├── Chart.yaml
    ├── values.yaml
    ├── secrets.enc.yaml        # SOPS-encrypted PowerDNS API key
    └── templates/
        ├── deployment.yaml    # Checksum annotation for auto-restart on Secret change
        ├── rbac.yaml          # Includes gateway.networking.k8s.io + namespaces permissions
        └── secret.yaml
```

## Configuration

| Value | Source | Description |
|-------|--------|-------------|
| `pdns.apiUrl` | `values-common.yaml` | PowerDNS API endpoint |
| `pdns.serverId` | `values-common.yaml` | PowerDNS server ID (`localhost`) |
| `pdns.apiKey` | `secrets.enc.yaml` | PowerDNS API key (encrypted) |
| `domainFilter` | `{env}/values.yaml` | Target domain filter |

Source is set to `gateway-httproute`, so DNS records are created automatically when HTTPRoute resources are applied.

> `butaco.net` is a personal domain. Replace it in `dev/values.yaml` and `prd/values.yaml`.

## Secrets

```bash
sops edit k8s/externalDNS/chart/secrets.enc.yaml
```

| Field | Description |
|-------|-------------|
| `pdns.apiKey` | PowerDNS HTTP API key |

## Notes

- RBAC includes `gateway.networking.k8s.io` group and `namespaces` resource, required for `gateway-httproute` source
- Deployment has a checksum annotation on the Secret so it restarts automatically when credentials change
