# ExternalDNS

Registers PowerDNS records from HTTPRoutes; ESO supplies the API key.

## Directory Structure

```
externalDNS/
├── values-common.yaml         # PowerDNS API endpoint (shared)
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
| `pdns.apiKey` | OpenBao via ESO | PowerDNS API key materialized as a Kubernetes Secret |
| `domainFilter` | `{env}/values.yaml` | Target domain filter |

The `gateway-httproute` source creates records; environments use distinct TXT
owner IDs.

> `butaco.net` is a personal domain. Replace it in each environment values file.

## Secrets

ESO fetches the PowerDNS API key from `k8s/external-dns/pdns`; plaintext is
never committed.

| Property | Description |
|----------|-------------|
| `api-key` | PowerDNS HTTP API key |

Seed it through the encrypted Ansible `openbao_secrets` list:

```bash
ansible-playbook ansible/playbooks/ops-openbao_seed_secrets.yaml
```

## Notes

- RBAC includes Gateway API and namespace access required by the source.
- A Secret checksum restarts the Deployment after credential changes.
