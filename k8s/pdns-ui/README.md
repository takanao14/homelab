# pdns-ui

Read-only browser for the PowerDNS authoritative zones, deployed on the prd
cluster and managed by ArgoCD.

Records are owned by dnscontrol (see the private plans repo), so this app is a
viewer only — it must never become a second source of truth. Read-only is
enforced in nginx, not merely by convention.

## Directory Structure

```
pdns-ui/
├── prd/values.yaml         # prd overrides (hostname, Gateway https listener)
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
browser ──https──► Envoy Gateway ──► pdns-ui pod (nginx)
                                       ├─ /        → vendored index.html
                                       └─ /api/*   → ns1:8081, X-API-Key injected
```

The webapp is pure client-side JavaScript and talks to the PowerDNS API on its
own origin. nginx injects the API key server-side, so the credential stays in
the pod and never reaches the browser.

## Read-only enforcement

`limit_except GET HEAD { deny all; }` on both locations. The injected key grants
full write access to PowerDNS, so this guard is the only thing standing between
the UI and zone modification — do not relax it to make an edit feature work. If
editing is ever wanted, that is a design decision to take against dnscontrol
ownership first.

Verified against the real image before deployment: `POST`/`PUT`/`PATCH`/`DELETE`
return `403` on both `/` and `/api/*`, `GET /api/*` is proxied with the key
attached, and the key does not appear in any served content.

## Vendoring

`chart/web/index.html` is [james-stevens/powerdns-webui](https://github.com/james-stevens/powerdns-webui)
(MIT), vendored rather than pulled at runtime so the third-party JavaScript that
reads the zone data is reviewable in git. The pinned tag and the SHA256 of the
vendored bytes live in `chart/web/REVISION`.

The app is self-contained: no external scripts, styles, or fonts, and exactly
one `fetch()`, to the PowerDNS API on its own origin. `sync.sh` refuses to
vendor a version that gained an external resource load.

### Updating

```bash
k8s/pdns-ui/chart/web/sync.sh            # fetch the ref recorded in REVISION
REF=v3.7 k8s/pdns-ui/chart/web/sync.sh   # move to a new tag
k8s/pdns-ui/chart/web/sync.sh --check    # what CI runs
```

Renovate watches the tag via the `# renovate:` comment in `REVISION` and opens a
PR that bumps `ref:` alone. That PR is deliberately incomplete: `vendor-sync` CI
compares the vendored bytes against the new ref and fails until `sync.sh` has
been run, so the recorded version and the actual file cannot drift apart. The
same check runs weekly, which also catches an upstream tag being re-pointed.

Do not edit `index.html` by hand — `--check` treats that as drift, which is the
point.

After any update, re-verify the read-only behaviour below before merging.

## Secrets

| OpenBao path | Property | Description |
|--------------|----------|-------------|
| `k8s/external-dns/pdns` | `api-key` | ns1's PowerDNS API key |

This reuses the entry external-dns already reads rather than seeding a second
copy. PowerDNS authoritative has a single, unscoped `api-key`, so a dedicated
`k8s/pdns-ui/*` path would hold the identical credential and create a second
place to forget during rotation. The trade-off is that the KV path name is
external-dns-flavoured; a neutral `k8s/shared/pdns` would read better but means
re-seeding OpenBao and editing a working external-dns ExternalSecret.

Because the prd `ClusterSecretStore` authenticates with one cluster-wide
OpenBao role that already carries the external-dns policy, no OpenBao seeding
or policy change is needed for this app.

The same key lives in `PDNS_PRIMARY_API_KEY` in
`ansible/inventories/homelab/host_vars/ns1.sops.yaml`; SOPS files are not
mirrored into OpenBao, so a rotation still has to update both.

## Notes

- The upstream webapp documents testing against PowerDNS 4.2.2, while ns1 runs
  the `auth-50` channel. Zone listing needs to be confirmed on first sync.
- `fsGroup: 101` is required. The nginx entrypoint renders
  `/etc/nginx/templates` into the `/etc/nginx/conf.d` emptyDir; without a
  group-writable mount it skips envsubst silently and nginx falls back to its
  stock config, dropping both the API proxy and the read-only guard.
- `NGINX_ENVSUBST_FILTER=^PDNS_` keeps envsubst away from nginx's own `$host` /
  `$uri` variables.
