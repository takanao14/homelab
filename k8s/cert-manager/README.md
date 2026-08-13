# cert-manager

Configures cert-manager wildcard certificates through Let's Encrypt DNS-01 and Cloudflare.

Managed by ArgoCD; secrets are injected by External Secrets Operator from
OpenBao (see [ADR-0012](../../docs/adr/0012-openbao-eso-cluster-rebuild-registration.md)).
Two Argo CD Applications are used:

- `cert-manager` — upstream chart (installs CRDs and the controller)
- `cert-manager-config` — this local chart (ClusterIssuer, Certificate, Secret)

## Directory Structure

```
cert-manager/
├── Chart.yaml
├── values.yaml               # Schema: email, domain (local config chart)
├── prd/
│   └── values.yaml           # domain: prd.butaco.net
├── controller/               # Values for the upstream cert-manager chart
│   ├── values.yaml           # Common: CRDs, DNS-01 resolvers, ServiceMonitor
│   └── prd/values.yaml       # cluster=prd ServiceMonitor relabeling
└── templates/
    ├── cluster-issuer.yaml              # letsencrypt-staging + letsencrypt-production
    ├── certificate.yaml                 # Wildcard cert: *.{domain}
    ├── cloudflare-external-secret.yaml  # ESO ExternalSecret for Cloudflare API token
    └── reference-grant.yaml             # Allows gateway-system to reference TLS secret
```

## How It Works

1. `ClusterIssuer` uses Cloudflare DNS-01 challenge to prove domain ownership
2. `Certificate` requests `*.{domain}` from letsencrypt-production
3. The issued certificate is stored as a Secret in `cert-manager` namespace
4. `ReferenceGrant` allows the shared Gateway in `gateway-system` to use the Secret for TLS termination

## Certificate

| Field | Value |
|-------|-------|
| DNS Name | `*.{domain}` (e.g. `*.prd.butaco.net`) |
| Secret name | `wildcard-{domain-dashes}-tls` (e.g. `wildcard-prd-butaco-net-tls`) |
| Namespace | `cert-manager` |
| Issuer | `letsencrypt-production` |

> `butaco.net` is a personal domain. Replace it in `prd/values.yaml`.

## Secrets

ESO fetches the Cloudflare API token from OpenBao; plaintext is never committed.

OpenBao KV path: `k8s/cert-manager/cloudflare`

| Property | Description |
|----------|-------------|
| `api-token` | Cloudflare API token with `Zone:DNS:Edit` permission |

Seed it through the encrypted Ansible `openbao_secrets` list and
`ops-openbao_seed_secrets.yaml`; do not use manual `bao kv put`.

## Notes

- `--dns01-recursive-nameservers=8.8.8.8:53,1.1.1.1:53` is set in the upstream cert-manager App to bypass internal DNS (PowerDNS) during ACME validation
- Both staging and production ClusterIssuers are created; the Certificate uses production
