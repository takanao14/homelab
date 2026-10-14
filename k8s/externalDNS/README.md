# ExternalDNS

Registers PowerDNS records for prd from HTTPRoutes and annotated LoadBalancer
Services; ESO supplies the API key. Sandbox has no external-dns: the API key is
not zone-scoped (ADR-0070).

## Directory Structure

```
externalDNS/
├── prd/
│   └── values.yaml            # PowerDNS endpoint, domainFilter: prd.butaco.net.
└── chart/                     # Local Helm chart
    ├── Chart.yaml
    ├── values.yaml
    └── templates/
        ├── deployment.yaml      # Reloader annotation for Secret-driven restarts
        ├── rbac.yaml            # Includes gateway.networking.k8s.io + namespaces permissions
        └── external-secret.yaml # ESO ExternalSecret for PowerDNS API key
```

## Configuration

| Value | Source | Description |
|-------|--------|-------------|
| `pdns.apiUrl` | `prd/values.yaml` | PowerDNS API endpoint |
| `pdns.serverId` | `prd/values.yaml` | PowerDNS server ID (`localhost`) |
| `pdns.apiKey` | OpenBao via ESO | PowerDNS API key materialized as a Kubernetes Secret |
| `domainFilter` | `prd/values.yaml` | Target domain filter |
| `policy` | `prd/values.yaml` | DNS record lifecycle policy |

The prd instance uses `policy: sync` and TXT owner ID `prd-cluster`.
Its domain filter and TXT ownership restrict deletion to its managed records.
Sandbox uses a separately managed wildcard record instead.

## Secrets

ESO fetches the PowerDNS API key from `k8s/external-dns/pdns`; plaintext is
never committed.

| Property | Description |
|----------|-------------|
| `api-key` | PowerDNS HTTP API key |

Seed it through the encrypted Ansible `openbao_secrets` list:

```bash
cd ansible
ansible-playbook playbooks/ops/openbao/seed_secrets.yaml
```

After rotating the key and waiting for ESO synchronization, restart external-dns
explicitly; the current [Reloader scope](../reloader/README.md#configuration)
does not cover its namespace.
