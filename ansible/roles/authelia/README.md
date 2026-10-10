# authelia Role

Deploys [Authelia](https://www.authelia.com/) as a root system Podman Quadlet
unit on the LLDAP host (ADR-0067). The portal is published through Caddy at
`https://auth.home.butaco.net`; prd Envoy Gateway calls
`/api/authz/ext-authz/` on `authelia_http_port` directly.

## Functionality

- Authenticates against LLDAP over LDAPS with the read-only
  `lldap_authelia_user` account, trusting `roles/sssd/files/lldap-server.pem`.
- Admits each `authelia_access_rules` domain to any of its groups with
  `one_factor`; every other request is denied.
- Issues one session cookie for `authelia_cookie_domain` with an absolute
  `authelia_session_expiration` lifetime and no remember-me.
- Disables password reset and change; users change passwords in LLDAP.
- Keeps sessions in memory, so a restart signs everyone out. SQLite under
  `authelia_config_dir` holds only regulation and second-factor state.

## Variables

### SOPS-managed variables

Store these in `inventories/homelab/group_vars/authelia.sops.yaml`. Each must be
a random string of at least 64 characters.

| Variable | Description |
|----------|-------------|
| `authelia_session_secret` | Session encryption secret |
| `authelia_storage_encryption_key` | SQLite encryption key; changing it makes the database unreadable |
| `authelia_reset_password_jwt_secret` | Required by Authelia even with password reset disabled |

`lldap_authelia_password` comes from `lldap.sops.yaml`.

### Defaults

| Variable | Default | Description |
|----------|---------|-------------|
| `authelia_version` | `4.39.28` | Renovate-managed image tag |
| `authelia_http_port` | `9091` | Portal and authz endpoint |
| `authelia_access_rules` | GPU Switch | Protected domains and admitted groups |
| `authelia_session_expiration` | `8h` | Absolute session lifetime |

## Operations

- **New protected host:** add an `authelia_access_rules` entry, run
  `ansible-playbook playbooks/services/authelia.yaml`, then point the host's
  forward auth at Authelia.
- **LDAPS certificate rotation:** after the lldap rotation steps, run this
  playbook too; Authelia rejects LDAPS until it has the new certificate.
- **Recovery:** rebuild the VM and run the lldap and authelia playbooks. Losing
  SQLite drops only regulation state.

### Health checks

```sh
systemctl is-active authelia
curl -fsS http://127.0.0.1:9091/api/health
```
