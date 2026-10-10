# lldap Role

Deploys [LLDAP](https://github.com/lldap/lldap) as a root system Podman Quadlet
unit and reconciles its users and groups from SOPS with the image's
`bootstrap.sh` (ADR-0067). SSSD clients read it over LDAPS; the web UI is
published through Caddy at `https://lldap.home.butaco.net`.

## Functionality

- Publishes LDAPS on `identity_ldaps_port` and HTTP on `lldap_http_port`.
  Plaintext LDAP is not published.
- Generates a self-signed LDAPS certificate for `identity_ldap_host` on first
  deployment and exports the public certificate to `roles/sssd/files/`.
- Defines POSIX custom attributes (`uidnumber`, `gidnumber`, `homedirectory`,
  `unixshell`, `sshpublickey`; `gidnumber` on groups).
- Creates every `lldap_groups` entry, a personal primary group per user whose
  GID equals the UID, every `lldap_users` entry, and the read-only
  `identity_ldap_bind_user` account for SSSD, and the read-only
  `lldap_authelia_user` account for Authelia.
- Runs the bootstrap with cleanup enabled: users, groups, and memberships not
  declared here are deleted. It then fails if any declared user is missing or
  differs in groups or UID.

## Variables

### SOPS-managed variables

Store these in `inventories/homelab/group_vars/lldap.sops.yaml`.

| Variable | Description |
|----------|-------------|
| `lldap_jwt_secret` | Web session signing secret |
| `lldap_key_seed` | Seed for the server key that protects stored passwords; changing it invalidates every password |
| `lldap_admin_password` | Built-in `admin` password; LLDAP applies it only when it creates the database |
| `lldap_bind_password` | SSSD lookup account password; must equal `sssd_ldap_bind_password` |
| `lldap_authelia_password` | Authelia lookup account password; read by the `authelia` role |
| `lldap_users` | Managed account list |

Each `lldap_users` entry accepts:

| Key | Required | Description |
|-----|----------|-------------|
| `username` | yes | Login name (`uid`) and personal group name |
| `uid` | yes | `uidNumber` and personal `gidNumber`, within `lldap_uid_min`–`lldap_uid_max` |
| `password` | yes | Initial password, set only when the account is created |
| `groups` | no | Authoritative membership from `lldap_groups` names or LLDAP built-ins such as `lldap_admin` |
| `name` | no | Display name; defaults to `username` |
| `email` | no | Defaults to `<username>@home.butaco.net` |
| `shell` | no | Defaults to `lldap_default_shell` |
| `ssh_public_keys` | no | List published as `sshPublicKey` |

### Defaults

| Variable | Default | Description |
|----------|---------|-------------|
| `lldap_version` | `v0.6.3` | Renovate-managed image tag |
| `lldap_groups` | 8 `lab-*` groups, GIDs 20001–20008 | The only definition of lab group names and GIDs |
| `lldap_uid_min` / `lldap_uid_max` | `10000` / `19999` | User UID range |
| `lldap_http_url` | `https://lldap.home.butaco.net` | Browser-facing URL |
| `lldap_cert_validity_days` | `365` | LDAPS certificate validity |
| `lldap_cert_rotate` | `false` | Replace the LDAPS certificate on this run |

## Operations

### Users

- **Onboarding:** add an `lldap_users` entry with an unused UID and run
  `ansible-playbook playbooks/services/lldap.yaml`. Linux login requires
  `lab-linux-users`; sudo additionally requires `lab-linux-admins`.
- **Password changes:** users change their own password in the web UI. Editing
  `password` in SOPS does not reset an existing account.
- **Offboarding:** remove the entry and run the playbook; the account and its
  personal group are deleted. Then run `sss_cache -u <user>` on SSSD hosts for
  immediate effect and end open SSH sessions.

### Certificate rotation

Run the playbook with `-e lldap_cert_rotate=true`, commit the exported
`roles/sssd/files/lldap-server.pem`, and run `playbooks/services/sssd.yaml`.
Clients reject LDAPS until they receive the new certificate.

### Recovery

Ansible declares every user and group, so rebuilding the VM and running the
playbook restores the directory. Only passwords changed after account creation
are lost; their users receive the SOPS initial password again. A rebuild also
generates a new certificate, so finish with the rotation steps above.

### Health checks

```sh
systemctl is-active lldap
curl -fsS -o /dev/null http://127.0.0.1:17170/
openssl s_client -connect ldap.home.butaco.net:636 </dev/null 2>/dev/null \
  | openssl x509 -noout -enddate
```
