# authentik Role

Deploys Authentik server, worker, PostgreSQL, and standalone LDAP and Proxy
Outposts as root system Podman Quadlet units. The deployment is based on the
[official Docker Compose reference](https://docs.goauthentik.io/compose.yml),
adapted for Podman.

## Functionality

- Depends on the `podman` role and creates a dedicated Podman network for
  container-name resolution.
- Deploys and enables five services: `authentik-postgresql`,
  `authentik-server`, `authentik-worker`, `authentik-ldap`, and
  `authentik-proxy`.
- Publishes the server HTTP port for Caddy, LDAPS for SSSD, and the Proxy
  Outpost HTTP port for Envoy Gateway.
- Deploys root-owned templates, blueprints, and mode-0600 environment files.
  The data, certificate, and PostgreSQL directories keep container-managed
  ownership. PostgreSQL requires its data directory to remain mode 0700 or
  0750; forcing root ownership or mode 0755 breaks subsequent starts.
- Copies static blueprints from `files/blueprints/` and renders managed users
  from the SOPS-encrypted `authentik_users` list.
- Uses the Authentik API to generate the LDAPS certificate, apply the managed
  users blueprint, grant LDAP search permission, and retrieve standalone
  Outpost tokens.

## Differences from upstream Compose

- The Docker socket is not mounted. LDAP and Proxy Outposts are explicit
  token-based Quadlet services.
- Port 9443 is not published. Caddy terminates TLS and connects to
  `authentik_http_port` over HTTP.
- Quadlet `EnvironmentFile=` does not perform Compose-style `${VAR}`
  substitution, so the environment template emits every variable name needed
  by PostgreSQL and Authentik.

## Variables

### SOPS-managed variables

Store these values in SOPS-encrypted group variables such as
`authentik.sops.yaml`.

| Variable | Required | Description |
|----------|----------|-------------|
| `authentik_pg_pass` | yes | PostgreSQL password for the Authentik database |
| `authentik_secret_key` | yes | Session and token signing key; rotation invalidates existing sessions |
| `authentik_bootstrap_email` | no | Initial `akadmin` email; defaults to an empty string |
| `authentik_bootstrap_password` | yes | Initial `akadmin` password |
| `authentik_bootstrap_token` | yes | API token used by this role; bootstrap creates it only on a fresh database |
| `authentik_test_user_password` | yes | Password for the disposable `sssdtest` account |
| `authentik_ldap_bind_password` | yes | Password for the `ldapbind` SSSD lookup account |
| `authentik_users` | yes | Managed account list rendered into `users.yaml` |

### Operator-facing defaults

| Variable | Default | Description |
|----------|---------|-------------|
| `authentik_version` | `2026.5.6` | Authentik server and Outpost image tag |
| `authentik_network` | `authentik` | Podman network name |
| `authentik_pg_db` / `authentik_pg_user` | `authentik` | PostgreSQL database and user |
| `authentik_base_dir` | `/opt/authentik` | Parent directory for data, templates, certificates, and blueprints |
| `authentik_pg_data_dir` | `/var/lib/authentik-postgresql` | PostgreSQL data directory |
| `authentik_env_file` | `/etc/authentik/authentik.env` | Shared PostgreSQL/server/worker environment file |
| `authentik_http_port` | `9000` | Server HTTP port published for Caddy |
| `authentik_ldap_outpost_name` | `homelab-ldap-outpost` | Must match `files/blueprints/ldap.yaml` |
| `authentik_ldaps_port` | `636` | Published LDAPS port; plaintext LDAP is not exposed |
| `authentik_ldap_cert_name` | `homelab-ldap` | Certificate name referenced by the LDAP blueprint |
| `authentik_ldap_cert_cn` | `ldap.home.butaco.net` | Generated certificate CN and SAN |
| `authentik_ldap_cert_validity_days` | `365` | Generated certificate validity |
| `authentik_proxy_outpost_name` | `homelab-proxy-outpost` | Must match `files/blueprints/proxy.yaml` |
| `authentik_proxy_http_port` | `9001` | Proxy Outpost port used by Envoy Gateway |
| `authentik_external_url` | `https://auth.home.butaco.net` | Browser-facing Authentik URL |

## Operations

Bootstrap variables affect only a fresh database without an admin user. If
bootstrap access is unavailable, create a time-limited recovery URL with:

```sh
podman exec authentik-worker ak create_recovery_key <minutes> akadmin
```

Do not rotate `authentik_secret_key` without a migration plan. Keep all
plaintext credentials in SOPS; rendered environment files are mode 0600.

Static provider, application, group, and Outpost configuration belongs in
`files/blueprints/`. Authentik watches the mounted `/blueprints/local`
directory and applies changed files. The role retrieves LDAP and Proxy Outpost
tokens from the API on every run, so they are not stored in SOPS.

`files/blueprints/test-users.yaml` creates the disposable `sssdtest` account
for SSSD validation. When the account is no longer needed, remove the
blueprint, its environment-template entry, and its SOPS variable together.

### Managed users

Each entry in `authentik_users` accepts:

| Key | Required | Description |
|-----|----------|-------------|
| `username` | yes | Login name and LDAP `cn` returned to SSSD |
| `password` | yes | Initial password |
| `groups` | no | Authoritative group list; removing a group revokes membership on the next apply |
| `name` | no | Display name; defaults to `username` |
| `email` | no | Defaults to `<username>@home.butaco.net` |
| `path` | no | Authentik user path; defaults to `users` |
| `is_active` | no | Defaults to `true`; set to `false` to disable the account |
| `ssh_public_key` | no | Published as `sshPublicKey` for `sss_ssh_authorizedkeys` |

The users blueprint creates each password once and reconciles profile, group,
path, and active-state fields on every apply. Changing `password` in
`authentik_users` does not reset an existing account; reset it in the Authentik
UI, or delete the account and rerun the playbook.

Passwords are passed through `!Env` from the mode-0600 environment file rather
than stored in the mode-0644 blueprint. The role explicitly reapplies the users
blueprint so deleted accounts are recreated, then verifies that every managed
account exists with a usable password.

### Proxy Outpost

`files/blueprints/proxy.yaml` defines forward-auth providers for Headlamp and
the GPU switch. The standalone Proxy Outpost serves both `prd` and `sandbox`.
It answers authorization checks but is not a reverse proxy in the request path;
each cluster's Envoy Gateway calls `authentik_proxy_http_port` directly over the
LAN through a `SecurityPolicy`.
