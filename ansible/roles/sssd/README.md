# sssd Role

Joins a Linux host to Authentik's LDAP Outpost (see `roles/authentik`) for
user/group lookup, password authentication, and SSH public key retrieval —
stage 6 of `docs/plans/identity-authentication-architecture.md`.

```text
Linux host -> NSS/PAM -> SSSD -> LDAPS -> Authentik LDAP Outpost
```

## Functionality

- Creates a **break-glass local account** (`sssd_breakglass_user`) with
  passwordless sudo and explicitly-listed SSH keys, *before* configuring
  anything that makes the host depend on Authentik. This is the documented
  escape hatch for when the identity provider is unavailable, re-asserted on
  every run rather than left to whatever cloud-init set at VM creation.
- Installs SSSD + `libnss-sss` / `libpam-sss` / `ldap-utils`.
- Deploys the Authentik LDAP certificate SSSD must trust
  (`files/authentik-ldap-ca.pem` — the `homelab-ldap` keypair the authentik
  role generates, CN/SAN `ldap.home.butaco.net`) and points
  `ldap_tls_cacert` at it, so `ldap_tls_reqcert = demand` still holds
  without a real CA chain. Note this is *not* Authentik's built-in
  self-signed certificate: that one carries a random
  `*.self-signed.goauthentik.io` SAN which no client dialling the LDAP
  service name can verify.
- Deploys `/etc/sssd/sssd.conf` (mode 0600 — it carries the LDAP bind
  password; SSSD refuses to start otherwise).
- Enables `pam_mkhomedir` via `pam-auth-update` so home directories are
  created on first login.
- Points sshd's `AuthorizedKeysCommand` at `sss_ssh_authorizedkeys`, so each
  user's `sshPublicKey` attribute in Authentik is the single source of truth
  for SSH keys (no per-host `authorized_keys` management).
- Restricts login to `sssd_allowed_login_group` (`lab-linux-users`) **and**
  to users still enabled in Authentik, via `access_provider = ldap` with an
  `ldap_access_filter` (see the disable-behaviour note below).
- Grants sudo to `sssd_sudo_group` (`lab-linux-admins`) through an
  Ansible-managed `/etc/sudoers.d/` file — Authentik's LDAP provider does
  not serve central sudo rules. Password is required rather than NOPASSWD,
  so sudo re-verifies the operator against Authentik.

## Authentik-specific schema notes

Authentik's LDAP Outpost does not present a stock RFC2307 layout:

- **The login name is `cn`, not `uid`.** `uid` holds an opaque hash
  (e.g. `059d2d63…`), so `ldap_user_name` must be `cn` or every lookup
  returns a hash as the username.
- Users are under `ou=users,<base>`, real groups under `ou=groups,<base>`,
  and each user additionally gets a "virtual group" under
  `ou=virtual-groups,<base>` whose `gidNumber` equals the user's own
  `uidNumber` (the usual Unix per-user group convention).
- `objectClass` is `user` / `group` (AD-style) rather than
  `posixAccount` / `posixGroup` alone, hence `ldap_schema = rfc2307bis`
  with explicit `ldap_user_object_class` / `ldap_group_object_class`.
- `uidNumber` / `gidNumber` are assigned by Authentik from the provider's
  `uid_start_number` / `gid_start_number` (10000, per 決定事項14).

## Variables

### Secrets (in `inventories/homelab/group_vars/sssd.sops.yaml`)

| Variable | Description |
|----------|-------------|
| `sssd_ldap_bind_password` | Password for `ldapbind`, the shared directory-lookup service account. Kept as its own `sssd`-group secret rather than read from `authentik.sops.yaml`, since group_vars only reach hosts in that group — the same value is set on the Authentik side via `authentik_ldap_bind_password`, so **both must be updated together** if rotated. |

### Break-glass (in `inventories/homelab/group_vars/sssd.yaml`)

`sssd_breakglass_authorized_keys` is deliberately **not** SOPS-encrypted:
recovering access during an outage must not depend on being able to decrypt
anything. Public keys are not secrets. An empty list skips key management
entirely, so an unset variable can never lock the account out.

### Non-secret variables (in `defaults/main.yaml`)

| Variable | Default | Description |
|----------|---------|-------------|
| `sssd_breakglass_user` | `breakglass` | Local account that survives an Authentik/LDAP outage |
| `sssd_breakglass_sudoers_file` | `/etc/sudoers.d/10-breakglass` | NOPASSWD sudo for the above |
| `sssd_ldap_uri` | `ldaps://ldap.home.butaco.net:636` | LDAPS only; plaintext 389 is not published |
| `sssd_ldap_search_base` | `dc=ldap,dc=home,dc=butaco,dc=net` | Must match the provider's Base DN |
| `sssd_ldap_bind_dn` | `cn=ldapbind,ou=users,…` | Shared lookup account |
| `sssd_allowed_login_group` | `lab-linux-users` | Only this group may log in |
| `sssd_sudo_group` | `lab-linux-admins` | Authentik group granted sudo |
| `sssd_sudoers_file` | `/etc/sudoers.d/60-lab-linux-admins` | Ansible-managed sudoers drop-in |
| `sssd_offline_credentials_expiration` | `2` | Days of offline login (決定事項16) |
| `sssd_entry_cache_timeout` | `600` | Seconds before a cached entry expires; caps how long a revoked group survives in `id`/initgroups, and therefore in sudo. SSSD's own default is `5400` |
| `sssd_ldap_ca_cert_path` | `/etc/ssl/certs/authentik-ldap-ca.pem` | Trusted LDAPS certificate |

## Notes

- **Disabling a user in Authentik does not remove them from LDAP.** The
  entry stays, group membership stays, and only the Authentik-specific
  `ak-active` attribute flips to `FALSE`. `access_provider = simple` (which
  only checks group membership) would therefore keep letting disabled users
  in — hence `access_provider = ldap` with
  `ldap_access_filter = (&(memberOf=…)(ak-active=TRUE))`. Verified on
  `sssdtest1`: disabling then re-enabling flips `pam_acct_mgmt` between
  `Permission denied` and `Success`.
- **Revocation is not instant, and there are two caches in the path.**
  Both expire on their own; neither needs a restart. Measured on `sssdtest1`
  (2026-08-17, `search_mode: cached`):
  1. The Authentik LDAP Outpost's cache — a group membership added in
     Authentik became visible to `ldapsearch` after ~1 min 15 s, and a removal
     after ~2 min 22 s.
  2. SSSD's cache on this host — bounded by `entry_cache_timeout`
     (`sssd_entry_cache_timeout`, set to 600s here; SSSD defaults to 5400s).

  So the ceiling on privilege revocation is roughly cache (1) + cache (2).
  Both are separate from `offline_credentials_expiration`, which governs
  offline login.

  **`getent group` and `id` do not agree during that window.** They are served
  from different caches: `getent group lab-linux-admins` listed `sssdtest` as a
  member while `id sssdtest` still omitted the group. **sudo follows `id`**
  (initgroups), so a member shown by `getent group` may not actually hold sudo
  yet — and, in the other direction, may still hold it after removal.
  For immediate revocation, invalidate the user's entry on the host:
  `sudo sss_cache -u <user>` (narrower than `-E`, and observed to take effect
  at once).

  **Lowering `entry_cache_timeout` does not shorten entries already cached.**
  SSSD stamps an expiration onto each entry when it writes it, using whatever
  timeout was in effect then, and the cache lives on disk
  (`/var/lib/sss/db/*.ldb`) so a restart does not clear it. Observed here: an
  entry refreshed at 17:02 carried the old 5400s stamp (expiring 18:32), and
  survived the 17:14 config change and service restart unchanged — a revoked
  group therefore stayed effective for another 78 minutes. After one
  `sss_cache -u sssdtest`, the replacement entry expired in 600s as configured,
  and `sssctl user-show` reported the same value for both the cache entry and
  initgroups (so `entry_cache_user_timeout` does not need to be set separately).
  **After changing this value, invalidate once** (`sss_cache -E`, or per user)
  or the new ceiling does not apply to existing entries. The role does this for
  you: the `sssd.conf` task notifies `Invalidate the sssd cache` alongside
  `Restart sssd`, so a config change discards the stale stamps. The certificate
  task only restarts — it has no reason to drop the cache.

  Use `sssctl user-show <user>` to read the actual expiration timestamps rather
  than inferring them.

  An earlier note here claimed group changes stayed invisible over LDAP until
  `authentik-ldap` was restarted. Re-measurement disproved that: the outpost
  cache expires on its own within a couple of minutes. Setting the provider's
  `search_mode` to `direct` would remove cache (1) entirely, but it makes every
  lookup a live query, so it was **not** adopted.
- `sssd_sudo_group` membership is independent of
  `sssd_allowed_login_group`: an admin needs to be in *both* to log in and
  then escalate, since the login filter only admits `lab-linux-users`.
- **`sss_cache -E` does not simulate an outage.** It marks entries stale, but
  SSSD keeps serving them (and `sss_ssh_authorizedkeys` keeps returning keys)
  while the LDAP server is unreachable — measured on `sssdtest1` with the
  outpost stopped. Only wiping `/var/lib/sss/db/*.ldb` with `sssd` stopped
  actually removes the identities, at which point LDAP users vanish
  entirely (`id: no such user`) until the outpost is back **and** `sssd` is
  restarted. Useful to know both for testing revocation and for judging what
  an outage really looks like.
- Offline credential caching covers short Authentik outages only. It is
  explicitly **not** a break-glass mechanism — local accounts and SSH keys
  remain for that (see the plan's 可用性と復旧 section). Verified on
  `sssdtest1`: with the outpost stopped and the SSSD cache wiped, the local
  cloud-init account still logged in over SSH and escalated via
  `sudo -n` (`/etc/sudoers.d/90-cloud-init-users`), while LDAP identities
  were completely unavailable.
- This role does not manage sudoers. Per the plan, sudo for
  `lab-linux-admins` stays in Ansible-managed sudoers, since Authentik's
  LDAP provider does not serve central sudo rules.
- The certificate in `files/` expires (1 year by default —
  `authentik_ldap_cert_validity_days` in the authentik role). It is public
  data, so it lives in git rather than being fetched from Authentik's API at
  run time: doing the latter would require handing every SSSD client host an
  Authentik admin API token, which inverts the trust boundary (these are the
  hosts being authenticated, not the identity provider). Guard against
  expiry by monitoring the LDAPS endpoint instead of relying on remembering
  to rotate — the plan's 可用性と復旧 section already lists
  「TLS証明書期限の監視」as a prerequisite. To rotate: delete the
  `homelab-ldap` certificate in Authentik, re-run the authentik playbook
  (which regenerates it), re-export it here, and re-run this playbook.
