# cert-manager

Local Helm chart that configures cert-manager to issue wildcard TLS certificates via Let's Encrypt DNS-01 challenge using Cloudflare.

Managed by ArgoCD with the helm-secrets plugin. Two ArgoCD Applications are used:
- `cert-manager` — upstream chart (installs CRDs and the controller)
- `cert-manager-config` — this local chart (ClusterIssuer, Certificate, Secret)

## Directory Structure

```
cert-manager/
├── Chart.yaml
├── values.yaml               # Schema: email, domain (local config chart)
├── dev/
│   └── values.yaml           # domain: dev.butaco.net
├── prd/
│   └── values.yaml           # domain: prd.butaco.net
├── controller/               # Values for the upstream cert-manager chart
│   ├── values.yaml           # Common: CRDs, DNS-01 resolvers, ServiceMonitor
│   ├── dev/values.yaml       # No dev-specific overrides
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

> `butaco.net` is a personal domain. Replace it in `dev/values.yaml` and `prd/values.yaml`.

## Secrets

The Cloudflare API token is fetched from OpenBao via ESO. It is not stored in this repository.

OpenBao KV path: `k8s/cert-manager/cloudflare`

| Property | Description |
|----------|-------------|
| `api-token` | Cloudflare API token with `Zone:DNS:Edit` permission |

To seed the secret into OpenBao, add it to the encrypted Ansible
`openbao_secrets` list and run `ops-openbao_seed_secrets.yaml`. Do not run
`bao kv put` manually; Ansible is the source of truth for seeded values.

## Notes

- `--dns01-recursive-nameservers=8.8.8.8:53,1.1.1.1:53` is set in the upstream cert-manager App to bypass internal DNS (PowerDNS) during ACME validation
- Both staging and production ClusterIssuers are created; the Certificate uses production
