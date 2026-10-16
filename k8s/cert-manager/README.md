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

`ClusterIssuer` validates Cloudflare DNS-01 challenges; `ReferenceGrant`
allows the shared Gateway to use the certificate Secret across namespaces.

## Certificate

| Field | Value |
|-------|-------|
| DNS Name | `*.{domain}` (e.g. `*.prd.butaco.net`) |
| Secret name | `wildcard-{domain-dashes}-tls` (e.g. `wildcard-prd-butaco-net-tls`) |
| Namespace | `cert-manager` |
| Issuer | `letsencrypt-production` |

## Secrets

ESO fetches the Cloudflare API token from
`k8s/cert-manager/cloudflare`; plaintext is never committed.

| Property | Description |
|----------|-------------|
| `api-token` | Cloudflare API token with `Zone:DNS:Edit` permission |

Seed it through the encrypted Ansible `openbao_secrets` list and
`playbooks/ops/openbao/seed_secrets.yaml`; do not use manual `bao kv put`.

## Notes

- The controller uses public recursive resolvers with `--dns01-recursive-nameservers-only`
  so DNS-01 self-checks observe public challenge records rather than the
  internal DNS view.
- Use the production issuer for the wildcard certificate. The chart also
  provides a staging issuer for issuance tests; it does not reference that
  issuer from the deployed Certificate.
