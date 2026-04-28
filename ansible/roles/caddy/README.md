# caddy Role

Installs and configures [Caddy](https://caddyserver.com/) as an HTTPS reverse proxy on Debian-based systems. Uses Cloudflare DNS-01 challenge for automatic TLS certificate issuance via Let's Encrypt.

## Functionality

- Creates a dedicated system user/group (`caddy`).
- Downloads Caddy binary with Cloudflare DNS plugin from the official Caddy download API.
- Sets `CAP_NET_BIND_SERVICE` to allow binding to port 443 without root.
- Deploys `/etc/caddy/Caddyfile` from a Jinja2 template.
- Deploys `/etc/caddy/caddy.env` with the Cloudflare API token.
- Deploys and enables a systemd unit.

## Variables

### Secrets (from `group_vars/caddy.sops.yaml`)

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
| `caddy_upstreams` | `[]` | List of reverse proxy upstreams (set in `group_vars/caddy.yaml`) |

`caddy_upstreams` structure:

```yaml
caddy_upstreams:
  - hostname: ns1.home.butaco.net
    backend: 192.168.10.233:8081
```

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
