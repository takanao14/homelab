# authentik Role

Deploys [Authentik](https://goauthentik.io/) (server + worker + PostgreSQL)
as root system Podman Quadlet units, based on the official
`docker-compose.yml` reference (https://docs.goauthentik.io/compose.yml),
adapted for Podman instead of Docker.

## Functionality

- Depends on the `podman` role (Docker CE is not used — see
  `docs/plans/identity-authentication-architecture.md` decision 18).
- Creates data/template/certs/PostgreSQL directories under `/opt/authentik`
  and `/var/lib/authentik-postgresql`.
- Deploys a dedicated Podman network (`authentik.network`) so the
  postgresql/server/worker containers resolve each other by container name.
- Deploys Quadlet `.container` units to `/etc/containers/systemd/`:
  `authentik-postgresql`, `authentik-server`, `authentik-worker`, and
  `authentik-ldap` (the standalone LDAP Outpost, LDAPS-only on
  `authentik_ldaps_port`).
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
- The `authentik-ldap` outpost's `AUTHENTIK_TOKEN` is the one case where
  this role *does* read from the API (`GET .../view_key/` on the token the
  Outpost object owns) rather than only deploying blueprints — fetched live
  every run instead of stored in SOPS, so it self-heals if Authentik ever
  regenerates it. This mirrors Authentik's own documented standalone-outpost
  workflow (copy the outpost's token into your deployment); it does not
  script around anything Authentik doesn't already support, unlike the
  akadmin email/password case above.
