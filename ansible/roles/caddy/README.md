# caddy Role

Installs and configures [Caddy](https://caddyserver.com/) as a reverse proxy on Debian-based systems. By default it terminates HTTPS, using the Cloudflare DNS-01 challenge for automatic TLS certificate issuance via Let's Encrypt. Setting `caddy_acme_enabled: false` turns a host into a plain HTTP front end instead.

Each site is either a reverse proxy (`caddy_upstreams`) or a redirect that Caddy answers itself (`caddy_redirects`).

## Functionality

- Creates a dedicated system user/group (`caddy`).
- Downloads Caddy binary with Cloudflare DNS plugin from the official Caddy download API.
- Sets `CAP_NET_BIND_SERVICE` to allow binding to ports 80/443 without root.
- Deploys `/etc/caddy/Caddyfile` from a Jinja2 template.
- Deploys `/etc/caddy/caddy.env` with the Cloudflare API token, and removes it again when ACME is disabled.
- Deploys and enables a systemd unit.

## ACME and HTTP-only hosts

With `caddy_acme_enabled: true` (the default) the Caddyfile carries a global
options block with the ACME issuer, and every site address is a bare hostname,
which triggers Caddy's Automatic HTTPS.

With `caddy_acme_enabled: false`:

- the global options block is omitted entirely;
- every site address is prefixed with `http://`, which serves the site on port
  80 and suppresses Automatic HTTPS, so Caddy never contacts an ACME CA;
- `cloudflare_api_token` is not required, `/etc/caddy/caddy.env` is not
  deployed, and any existing copy is removed so the credential does not linger.

The systemd unit references the environment file as `EnvironmentFile=-...`, so
Caddy still starts when the file is absent.

## Variables

### Secrets (from `group_vars/caddy.sops.yaml`)

Only required when `caddy_acme_enabled` is `true`.

| Variable | Description |
|----------|-------------|
| `cloudflare_api_token` | Cloudflare API token with `Zone:DNS:Edit` permission |
| `caddy_acme_email` | Email address for Let's Encrypt account (expiry notifications) |

### Non-secret variables

| Variable | Default | Description |
|----------|---------|-------------|
| `caddy_binary` | `/usr/local/bin/caddy` | Binary path |
| `caddy_config` | `/etc/caddy/Caddyfile` | Caddyfile path |
| `caddy_env_file` | `/etc/caddy/caddy.env` | Environment file path |
| `caddy_acme_enabled` | `true` | Issue certificates via ACME. `false` serves every site over plain HTTP |
| `caddy_acme_ca` | Let's Encrypt production | ACME CA directory endpoint |
| `caddy_upstreams` | `[]` | List of reverse proxy upstreams (set in `group_vars/caddy.yaml`) |
| `caddy_redirects` | `[]` | List of redirect-only sites (set in `group_vars/caddy.yaml`) |

`caddy_upstreams` structure:

```yaml
caddy_upstreams:
  - hostname: ns1.home.butaco.net
    backend: 192.168.10.233:8081
  # HTTPS upstream with a self-signed cert (e.g. TrueNAS):
  - hostname: truenas-ui.home.butaco.net
    backend: 192.168.20.10:443
    scheme: https        # optional, defaults to "http"
    tls_insecure: true   # optional, skip cert verification on the Caddy->upstream leg
```

| Upstream field | Default | Description |
|----------------|---------|-------------|
| `hostname` | – | Public hostname Caddy serves (Let's Encrypt cert) |
| `backend` | – | Upstream `host:port` |
| `scheme` | `http` | Upstream scheme (`http` or `https`) |
| `tls_insecure` | `false` | Skip TLS verification for an `https` upstream using a self-signed cert |

`caddy_redirects` structure:

```yaml
caddy_redirects:
  - hostname: www2.home.butaco.net
    target: https://www.prd.butaco.net
  # Permanent (301) redirect:
  - hostname: old.home.butaco.net
    target: https://new.prd.butaco.net
    code: permanent
```

| Redirect field | Default | Description |
|----------------|---------|-------------|
| `hostname` | – | Public hostname Caddy serves |
| `target` | – | Redirect target origin. The request URI is appended, so paths and query strings are preserved |
| `code` | `temporary` | `temporary` (302), `permanent` (301), or an explicit 3xx status code |

`code` defaults to `temporary` on purpose: browsers cache a 301 indefinitely, so
a permanent redirect is hard to take back if the hostname is later repurposed.
Start with the default and switch to `permanent` once the mapping is settled.

## Secrets Setup

```bash
sops edit ansible/inventories/homelab/group_vars/caddy.sops.yaml
```

## Dependencies

`community.general` collection (for `capabilities` module).

## Usage

```bash
ansible-playbook playbooks/caddy.yaml
```

The playbook's `sync_dns` play also writes one `home.butaco.net` A record per
site into `dns-record/home-zone.js` in the `homelab-private` repository, and
commits it. Both `caddy_upstreams` and `caddy_redirects` are included, so a new
site needs no manual DNS work. Run only that part with `--tags sync_dns`, or
skip it with `--tags caddy`.
