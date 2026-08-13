# pdns-ui

Read-only PowerDNS authoritative-zone browser for prd and sandbox.

dnscontrol owns records; nginx enforces viewer-only access to prevent a second
source of truth.

## Directory Structure

```
pdns-ui/
├── prd/values.yaml         # prd overrides (hostname, Gateway https listener)
├── sandbox/values.yaml     # sandbox overrides (hostname, Gateway http listener, ADR-0010)
└── chart/
    ├── Chart.yaml
    ├── values.yaml         # hostname, image, PowerDNS backend, OpenBao path
    ├── web/                # vendored upstream webapp (see Vendoring)
    │   ├── index.html
    │   ├── REVISION        # pinned tag + sha256, read by sync.sh and Renovate
    │   └── sync.sh         # fetch / --check the vendored copy
    └── templates/
        ├── configmap-web.yaml    # index.html as a ConfigMap
        ├── configmap-nginx.yaml  # nginx vhost template (read-only guard)
        ├── external-secret.yaml  # ESO → PowerDNS API key
        ├── deployment.yaml
        ├── service.yaml          # ClusterIP
        └── httproute.yaml        # HTTPRoute → shared-gateway-envoy
```

## Architecture

```
browser ──https (prd) / http (sandbox)──► Envoy Gateway ──► pdns-ui pod (nginx)
                                                              ├─ /        → vendored index.html
                                                              └─ /api/*   → ns1:8081, X-API-Key injected
```

The client calls its own origin; nginx injects the API key in-pod.

## Read-only enforcement

Both locations allow only GET/HEAD. The key is unscoped, so never relax this
guard without revisiting dnscontrol ownership.

Verified: write verbs return 403, API reads are proxied, and served content does
not expose the key.

## Vendoring

`chart/web/index.html` vendors MIT-licensed
[powerdns-webui](https://github.com/james-stevens/powerdns-webui) for review.
`REVISION` pins its tag and SHA-256.

The app is self-contained and same-origin; `sync.sh` rejects external resources.

### Updating

```bash
k8s/pdns-ui/chart/web/sync.sh            # fetch the ref recorded in REVISION
REF=v3.7 k8s/pdns-ui/chart/web/sync.sh   # move to a new tag
k8s/pdns-ui/chart/web/sync.sh --check    # what CI runs
```

Renovate bumps `ref:`; vendor-sync CI then requires refreshed matching bytes and
also detects retagged upstream releases.

Do not edit `index.html` by hand; `--check` rejects drift.

After any update, re-verify the read-only behaviour below before merging.

## Secrets

| OpenBao path | Property | Description |
|--------------|----------|-------------|
| `k8s/external-dns/pdns` | `api-key` | ns1's PowerDNS API key |

This reuses external-dns's unscoped key to avoid duplicate rotation points.

Both environment roles already carry the external-dns policy, so no new OpenBao
seed or policy is required.

Rotation must update both OpenBao and `ns1.sops.yaml`; SOPS is not mirrored.

## Notes

- Upstream tested 4.2.2 while ns1 uses `auth-51`; verify zone listing after updates.
- `fsGroup: 101` lets nginx render the proxy and read-only guard into emptyDir.
- `NGINX_ENVSUBST_FILTER=^PDNS_` keeps envsubst away from nginx's own `$host` /
  `$uri` variables.
