# sssd Role

Connects Ubuntu 24.04/26.04 and Rocky Linux 9 hosts to LLDAP for NSS/PAM
lookups, password authentication, and SSH public keys (ADR-0067).

```text
Linux host -> NSS/PAM -> SSSD -> LDAPS -> LLDAP
```

## Functionality

- Creates the passwordless-sudo break-glass account before LDAP integration.
- Installs SSSD, NSS/PAM integration, LDAP tools, and the LLDAP certificate.
- Deploys `/etc/sssd/sssd.conf` with mode `0600` and enables
  `pam_mkhomedir`.
- Retrieves SSH keys from LLDAP through `sss_ssh_authorizedkeys`.
- Allows members of `sssd_allowed_login_group` to log in.
- Grants password-protected sudo to `sssd_sudo_group` through a managed
  sudoers file.

The endpoint, Base DN, bind account, and login/sudo group names come from the
`identity_*` variables in `group_vars/all.yaml`, which the `lldap` role reads
from the same definitions (ADR-0050).

`files/lldap-server.pem` is the self-signed certificate that the `lldap` role
generates for `identity_ldap_host` and exports here on first deployment.

## LLDAP schema

Users live under `ou=people` as `posixAccount` with `uid` login names; groups
live under `ou=groups` as `groupOfUniqueNames` with `uniqueMember`. LLDAP has no
native POSIX attributes, so `uidNumber`, `gidNumber`, `homeDirectory`,
`unixShell`, and `sshPublicKey` are custom attributes set by the `lldap` role.
The search filters skip entries without them, which hides the bind account.

## Variables

### Secret

`sssd_ldap_bind_password` lives in
`inventories/homelab/group_vars/sssd.sops.yaml`. It must match
`lldap_bind_password`; rotate both together.

### Break-glass keys

Set `sssd_breakglass_authorized_keys` in
`inventories/homelab/group_vars/sssd.yaml`. Public keys remain unencrypted so
recovery does not depend on SOPS. An empty list leaves existing keys unchanged.

### Defaults

| Variable | Default | Purpose |
|---|---|---|
| `sssd_breakglass_user` | `breakglass` | Local recovery account |
| `sssd_breakglass_sudoers_file` | `/etc/sudoers.d/10-breakglass` | Recovery-account sudo |
| `sssd_ldap_uri` | `ldaps://{{ identity_ldap_host }}:{{ identity_ldaps_port }}` | LDAP endpoint |
| `sssd_ldap_search_base` | `{{ identity_ldap_base_dn }}` | Provider Base DN |
| `sssd_ldap_bind_dn` | `uid={{ identity_ldap_bind_user }},ou=people,…` | Lookup account |
| `sssd_allowed_login_group` | `{{ identity_linux_login_group }}` | Login group |
| `sssd_sudo_group` | `{{ identity_linux_sudo_group }}` | Sudo group |
| `sssd_sudoers_file` | `/etc/sudoers.d/60-lab-linux-admins` | Managed sudoers file |
| `sssd_offline_credentials_expiration` | `2` | Offline-login days |
| `sssd_entry_cache_timeout` | `600` | Host cache lifetime in seconds |
| `sssd_ldap_ca_cert_path` | `/etc/ssl/certs/…` (Ubuntu), `/etc/openldap/certs/…` (Rocky) | Trusted LDAP certificate |

Ubuntu enables home creation through `pam-auth-update`. Rocky Linux selects
the SSSD `authselect` profile with `with-mkhomedir` and runs `oddjobd`. On a host
not yet managed by authselect, the role verifies the PAM and glibc packages
before allowing authselect's required initial overwrite.

## Operational notes

### Access and revocation

LLDAP has no disabled state. `ldap_access_filter` requires membership in
`sssd_allowed_login_group`, so removing that membership or deleting the user
revokes login.

`sssd_sudo_group` does not grant login access. Administrators must also belong
to `sssd_allowed_login_group`.

### Caching

LLDAP answers from its database, so revocation is bounded by the SSSD cache:
`sssd_entry_cache_timeout` (600s; upstream default 5400s). `getent group` and `id` may
temporarily disagree because they use different caches; sudo follows `id`.
Run `sudo sss_cache -u <user>` for immediate host-side invalidation.

Lowering `entry_cache_timeout` does not shorten existing on-disk entries. The
role runs `sss_cache -E` after `sssd.conf` changes. Use
`sssctl user-show <user>` to inspect expiry timestamps.

### Outages and recovery

`sss_cache -E` only marks entries stale; cached identities and SSH keys remain
available during an LDAP outage. Removing `/var/lib/sss/db/*.ldb` while SSSD is
stopped removes them until LDAP returns and SSSD restarts.

Offline credentials cover short outages but are not break-glass access. The
local account and its SSH keys remain the recovery path.

### Certificate rotation

The committed LDAP certificate is public data. Rotate it with
`playbooks/services/lldap.yaml -e lldap_cert_rotate=true`, which replaces the
key pair and exports the new certificate here, then run this role's playbook
so clients trust it.
