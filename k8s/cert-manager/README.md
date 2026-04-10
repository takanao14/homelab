# cert-manager

Local Helm chart that configures cert-manager to issue wildcard TLS certificates via Let's Encrypt DNS-01 challenge using Cloudflare.

Managed by ArgoCD with the helm-secrets plugin. Two ArgoCD Applications are used:
- `cert-manager` — upstream chart (installs CRDs and the controller)
- `cert-manager-config` — this local chart (ClusterIssuer, Certificate, Secret)

## Directory Structure

```
cert-manager/
├── Chart.yaml
├── values.yaml               # Schema: email, domain, cloudflare.apiToken
├── secrets.enc.yaml          # SOPS-encrypted Cloudflare API token
├── dev/
│   └── values.yaml           # domain: dev.butaco.net
├── prd/
│   └── values.yaml           # domain: prd.butaco.net
└── templates/
    ├── cluster-issuer.yaml   # letsencrypt-staging + letsencrypt-production
    ├── certificate.yaml      # Wildcard cert: *.{domain}
    ├── cloudflare-secret.yaml
    └── reference-grant.yaml  # Allows gateway-system to reference TLS secret
```

## How It Works

1. `ClusterIssuer` uses Cloudflare DNS-01 challenge to prove domain ownership
2. `Certificate` requests `*.{domain}` from letsencrypt-production
3. The issued certificate is stored as a Secret in `cert-manager` namespace
4. `ReferenceGrant` allows the `shared-gateway` in `gateway-system` to use the Secret for TLS termination

## Certificate

| Field | Value |
|-------|-------|
| DNS Name | `*.{domain}` (e.g. `*.prd.butaco.net`) |
| Secret name | `wildcard-{domain-dashes}-tls` (e.g. `wildcard-prd-butaco-net-tls`) |
| Namespace | `cert-manager` |
| Issuer | `letsencrypt-production` |

> `butaco.net` is a personal domain. Replace it in `dev/values.yaml` and `prd/values.yaml`.

## Secrets

`secrets.enc.yaml` contains the Cloudflare API token encrypted with SOPS + Age.

```bash
# Edit secrets
sops edit k8s/cert-manager/secrets.enc.yaml
```

Required secret fields:

| Field | Description |
|-------|-------------|
| `cloudflare.apiToken` | Cloudflare API token with `Zone:DNS:Edit` permission |

## Notes

- `--dns01-recursive-nameservers=8.8.8.8:53,1.1.1.1:53` is set in the upstream cert-manager App to bypass internal DNS (PowerDNS) during ACME validation
- Both staging and production ClusterIssuers are created; the Certificate uses production
