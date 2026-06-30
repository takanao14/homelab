# ExternalDNS

Automatically registers DNS records in PowerDNS from Kubernetes HTTPRoute
resources. Managed by ArgoCD, with the PowerDNS API key supplied by ESO.

## Directory Structure

```
externalDNS/
├── values-common.yaml         # PowerDNS API endpoint (shared)
├── dev/
│   └── values.yaml            # domainFilter: dev.butaco.net.
├── prd/
│   └── values.yaml            # domainFilter: prd.butaco.net.
├── sandbox/
│   └── values.yaml            # domainFilter: sandbox.butaco.net.
└── chart/                     # Local Helm chart
    ├── Chart.yaml
    ├── values.yaml
    └── templates/
        ├── deployment.yaml      # Checksum annotation for auto-restart on Secret change
        ├── rbac.yaml            # Includes gateway.networking.k8s.io + namespaces permissions
        └── external-secret.yaml # ESO ExternalSecret for PowerDNS API key
```

## Configuration

| Value | Source | Description |
|-------|--------|-------------|
| `pdns.apiUrl` | `values-common.yaml` | PowerDNS API endpoint |
| `pdns.serverId` | `values-common.yaml` | PowerDNS server ID (`localhost`) |
| `pdns.apiKey` | `secrets.enc.yaml` | PowerDNS API key (encrypted) |
| `domainFilter` | `{env}/values.yaml` | Target domain filter |

Source is set to `gateway-httproute`, so DNS records are created automatically
when HTTPRoute resources are applied. Each environment uses a distinct TXT
owner ID.

> `butaco.net` is a personal domain. Replace it in each environment values file.

## Secrets

The PowerDNS API key is fetched from OpenBao via ESO. It is not stored in this repository.

OpenBao KV path: `k8s/external-dns/pdns`

| Property | Description |
|----------|-------------|
| `api-key` | PowerDNS HTTP API key |

To seed the secret into OpenBao:

```bash
bao kv put secret/k8s/external-dns/pdns api-key=<key>
```

## Notes

- RBAC includes `gateway.networking.k8s.io` group and `namespaces` resource, required for `gateway-httproute` source
- Deployment has a checksum annotation on the Secret so it restarts automatically when credentials change
