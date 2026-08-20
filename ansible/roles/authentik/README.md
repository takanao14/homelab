# authentik Role

Deploys [Authentik](https://goauthentik.io/) (server + worker + PostgreSQL)
as root system Podman Quadlet units, based on the official
`docker-compose.yml` reference (https://docs.goauthentik.io/compose.yml),
adapted for Podman instead of Docker.

## Functionality

- Depends on the `podman` role (Docker CE is not used — see
  `docs/plans/identity-authentication-architecture.md` decision 18).
- Creates data/template/certs/PostgreSQL directories under `/opt/authentik`
  and `/var/lib/authentik-postgresql`. **Only the template, blueprint, and
  `/etc/authentik` directories have their ownership managed** (root:root 0755).
  `data`, `certs`, and the PostgreSQL data directory are created but their
  owner and mode are left alone, because the containers take them over on first
  use — the authentik image ends up owning them as uid 1000, and PostgreSQL as
  uid 70 with mode 0700. Asserting root:root 0755 on every run would revert
  that, and PostgreSQL refuses to start unless its data directory is 0700 or
  0750, so managing them would take the stack down on the second apply.
- Deploys a dedicated Podman network (`authentik.network`) so the
  postgresql/server/worker containers resolve each other by container name.
- Deploys Quadlet `.container` units to `/etc/containers/systemd/`:
  `authentik-postgresql`, `authentik-server`, `authentik-worker`,
  `authentik-ldap` (the standalone LDAP Outpost, LDAPS-only on
  `authentik_ldaps_port`), and `authentik-proxy` (the standalone Proxy
  Outpost on `authentik_proxy_http_port`).
- Deploys `/etc/authentik/authentik.env` (mode 0600) with the secrets shared
  across the three containers, mirroring upstream's `env_file: .env`
  pattern (each container reads only the variable names it recognizes).
- Deploys `files/blueprints/*.yaml` to `authentik_blueprints_dir`, mounted
  into server/worker at `/blueprints/local`. Authentik's own blueprint
  system (not this role) watches that directory and auto-discovers/applies
  changes — this is Authentik's native declarative-config mechanism, so
  provider/application/group configuration belongs in blueprint files here
  rather than being scripted through the API by this role.
- Enables and starts all three services.

## Differences from the upstream Docker Compose reference

- **No Docker socket.** The upstream worker container mounts
  `/var/run/docker.sock` so Authentik can auto-provision "Docker" type
  outposts. This role omits it: outposts (e.g. the planned LDAP Outpost)
  are deployed as their own explicit, token-based containers instead.
- **HTTPS port not published.** Only the HTTP port (`authentik_http_port`,
  default 9000) is published; TLS is terminated externally by Caddy per the
  identity-authentication-architecture plan. The container's self-signed
  9443 port is unused.
- **No compose-style `${VAR}` substitution.** Quadlet's `EnvironmentFile=`
  passes variables through as-is, so `authentik.env` defines the exact
  variable names each container expects (e.g. both `POSTGRES_PASSWORD` and
  `AUTHENTIK_POSTGRESQL__PASSWORD` for the same secret value) instead of
  relying on compose interpolating one `PG_PASS` into several places.

## Variables

### Secrets (must be set via SOPS-encrypted group_vars, e.g. `authentik.sops.yaml`)

| Variable | Description |
|----------|-------------|
| `authentik_pg_pass` | PostgreSQL password for the `authentik` role/database |
| `authentik_secret_key` | `AUTHENTIK_SECRET_KEY` (session/token signing key) |
| `authentik_bootstrap_email` | Initial `akadmin` email (`AUTHENTIK_BOOTSTRAP_EMAIL`) — treated as sensitive in this repo and kept in SOPS rather than `defaults/main.yaml` |
| `authentik_bootstrap_password` | Initial `akadmin` password (`AUTHENTIK_BOOTSTRAP_PASSWORD`) |
| `authentik_bootstrap_token` | Initial `akadmin` API token (`AUTHENTIK_BOOTSTRAP_TOKEN`) — usable for further API-driven configuration (OIDC providers, groups, applications) once the containers are up |
| `authentik_users` | List of accounts rendered into the `users.yaml` blueprint — see below |

### Non-secret variables (in `defaults/main.yaml`)

| Variable | Default | Description |
|----------|---------|-------------|
| `authentik_version` | `2026.5.6` | `ghcr.io/goauthentik/server` image tag |
| `authentik_network` | `authentik` | Podman network name |
| `authentik_pg_db` / `authentik_pg_user` | `authentik` | PostgreSQL DB/user |
| `authentik_base_dir` | `/opt/authentik` | Parent of data/templates/certs dirs |
| `authentik_pg_data_dir` | `/var/lib/authentik-postgresql` | PostgreSQL data directory |
| `authentik_env_file` | `/etc/authentik/authentik.env` | Secrets env file path |
| `authentik_blueprints_dir` | `/opt/authentik/blueprints` | Deployed `files/blueprints/*.yaml`, mounted at `/blueprints/local` |
| `authentik_http_port` | `9000` | Host port published for the server container |
| `authentik_ldap_image` | `ghcr.io/goauthentik/ldap` | LDAP Outpost image |
| `authentik_ldap_outpost_name` | `homelab-ldap-outpost` | Must match the Outpost name in `files/blueprints/ldap.yaml` |
| `authentik_ldaps_port` | `636` | Host port published for LDAPS; plaintext 389 is not exposed |
| `authentik_proxy_image` | `ghcr.io/goauthentik/proxy` | Proxy Outpost image |
| `authentik_proxy_outpost_name` | `homelab-proxy-outpost` | Must match the Outpost name in `files/blueprints/proxy.yaml` |
| `authentik_proxy_http_port` | `9001` | Host port published for the Proxy Outpost; mirrored by `forwardAuth.outpost.port` in `k8s/headlamp/chart/values.yaml`. Not 9100 — that is node_exporter, which scrapes this host |

## Dependencies

`podman` (see `meta/main.yaml`).

## Notes

- First deploy requires `authentik_pg_pass` and `authentik_secret_key` to
  already be set (task #8 in the identity-authentication-architecture work:
  generate once, store SOPS-encrypted, never rotate by re-running with a
  different value without a migration plan — `AUTHENTIK_SECRET_KEY`
  rotation invalidates all existing sessions).
- `AUTHENTIK_BOOTSTRAP_*` vars are read on every startup but only take
  effect while no admin user exists in the database yet. On a genuinely
  fresh deploy (empty database) they apply as documented. On this
  particular `authentik1` instance, Authentik's own default blueprint had
  already created `akadmin` (placeholder email, no usable password) on the
  very first container boot, before this role's bootstrap env vars were
  added — so the email/password vars did not apply retroactively. Regained
  access via the officially documented break-glass command instead:
  `podman exec authentik-worker ak create_recovery_key <minutes> akadmin`
  (note: the unit is minutes, not days, despite some upstream examples
  suggesting otherwise), which prints a one-time browser login link.
- `AUTHENTIK_BOOTSTRAP_PASSWORD` (plaintext) is used instead of the
  upstream-recommended `AUTHENTIK_BOOTSTRAP_PASSWORD_HASH`, consistent
  with how this repo already stores other plaintext credentials at rest
  in SOPS (e.g. `openbao_userpass_users[].password`). SOPS+AGE encryption
  at rest is this repo's accepted security boundary for such secrets.
- OIDC provider/application/group configuration is added as blueprint YAML
  under `files/blueprints/`, applied by Authentik itself (see above) — not
  scripted via the API from Ansible. `files/blueprints/groups.yaml` covers
  the groups already decided in the identity-authentication-architecture
  plan's "グループと権限" table. `files/blueprints/ldap.yaml` covers the
  LDAP Provider/Application/PolicyBinding/Outpost. OIDC providers/
  applications are deferred until redirect URIs are known (Headlamp/sandbox
  not deployed yet). `files/blueprints/test-users.yaml` creates a
  disposable `sssdtest` user (in `lab-linux-users`) for stage 6 SSSD
  validation against `sssdtest1` — delete it once that PoC is done. Its
  password is set via the blueprint `password` attr (plaintext, hashed by
  Authentik on save) sourced with `!Env TEST_USER_PASSWORD` from
  `authentik.env` rather than embedded in the 0644 blueprints directory.
- Real accounts are the one blueprint this role renders itself:
  `templates/blueprints/users.yaml.j2` is templated from the SOPS-encrypted
  `authentik_users` list, because usernames and group membership are data,
  not a fixed file. Each list item accepts:

  | Key | Required | Description |
  |-----|----------|-------------|
  | `username` | yes | Login name. Authentik LDAP returns `cn`, so this is also the UNIX login name on SSSD hosts |
  | `password` | yes | **Initial** password to hand to the person (see below) |
  | `groups` | no | Group names, matched with `!Find`. Authoritative — dropping one revokes it on the next apply |
  | `name` | no | Display name, defaults to `username` |
  | `email` | no | Defaults to `<username>@home.butaco.net` |
  | `is_active` | no | Defaults to `true`; set `false` to offboard without deleting |
  | `ssh_public_key` | no | Published as `sshPublicKey` for `sss_ssh_authorizedkeys` |

  Each user renders two blueprint entries. The first uses `state: created`
  and carries only the password, which authentik skips once the account
  exists ("Instance exists, skipping") — so the value really is an *initial*
  password and a reapply never resets one the person has changed. The second
  entry omits `password` and reconciles profile, groups and `is_active` on
  every apply. Passwords reach the blueprint through `!Env` from
  `authentik.env` (0600) rather than being rendered into the blueprints
  directory (0644).

  Rotating a password in `authentik_users` therefore does **not** change an
  existing account. Reset it in the Authentik UI, or delete the user and
  reapply to re-run the creation path.

  The role applies this blueprint explicitly through
  `POST /api/v3/managed/blueprints/{pk}/apply/` instead of relying on
  Authentik's file-hash discovery, which only reapplies when the rendered
  content changes — without it, deleting an account and rerunning would leave
  the account gone. A final task then asserts that every listed user exists
  with a usable password, because `!Env` returns `None` for a variable the
  worker cannot see and would otherwise store an unusable password silently
  and permanently.
- The `authentik-ldap` outpost's `AUTHENTIK_TOKEN` is the one case where
  this role *does* read from the API (`GET .../view_key/` on the token the
  Outpost object owns) rather than only deploying blueprints — fetched live
  every run instead of stored in SOPS, so it self-heals if Authentik ever
  regenerates it. This mirrors Authentik's own documented standalone-outpost
  workflow (copy the outpost's token into your deployment); it does not
  script around anything Authentik doesn't already support, unlike the
  akadmin email/password case above. `authentik-proxy` uses the same
  mechanism for its own token.
- `files/blueprints/proxy.yaml` covers the Proxy Outpost used for Headlamp
  forward auth (stage 8b-1). Both providers use `mode: forward_single`, so
  the outpost only answers authorization checks — it is **not** a reverse
  proxy in the request path, and therefore is not published through Caddy.
  Each cluster's Envoy Gateway calls `authentik_proxy_http_port` directly
  over the LAN via a `SecurityPolicy` (`k8s/headlamp/chart/templates/`).
  A single outpost serves prd and sandbox so a sandbox rebuild never
  touches it.
