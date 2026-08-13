# cert-manager

Issues wildcard certificates through Let's Encrypt DNS-01 and Cloudflare.

Argo CD uses two Applications; ESO injects secrets from OpenBao
([ADR-0012](../../docs/adr/0012-openbao-eso-cluster-rebuild-registration.md)):

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

1. `ClusterIssuer` completes Cloudflare DNS-01 validation.
2. `Certificate` stores `*.{domain}` in the `cert-manager` namespace.
3. `ReferenceGrant` permits the shared Gateway to use the TLS Secret.

## Certificate

| Field | Value |
|-------|-------|
| DNS Name | `*.{domain}` (e.g. `*.prd.butaco.net`) |
| Secret name | `wildcard-{domain-dashes}-tls` (e.g. `wildcard-prd-butaco-net-tls`) |
| Namespace | `cert-manager` |
| Issuer | `letsencrypt-production` |

> `butaco.net` is a personal domain. Replace it in `prd/values.yaml`.

## Secrets

ESO fetches the Cloudflare API token from
`k8s/cert-manager/cloudflare`; plaintext is never committed.

| Property | Description |
|----------|-------------|
| `api-token` | Cloudflare API token with `Zone:DNS:Edit` permission |

Seed it through the encrypted Ansible `openbao_secrets` list and
`ops-openbao_seed_secrets.yaml`; do not use manual `bao kv put`.

## Notes

- Public recursive resolvers bypass internal DNS during ACME validation.
- Both issuers are created; the Certificate uses production.
