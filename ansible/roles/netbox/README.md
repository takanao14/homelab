# netbox Role

Deploys [NetBox](https://github.com/netbox-community/netbox) IPAM/DCIM from the
upstream [netbox-docker](https://github.com/netbox-community/netbox-docker)
images as rootful Podman containers managed by systemd Quadlet (ADR-0046).

## Functionality

- Deploys the `netbox` Podman network and five Quadlet units: `netbox`
  (Granian), `netbox-worker` (`manage.py rqworker`), `netbox-postgres`,
  `netbox-redis` (task queue) and `netbox-redis-cache`.
- Renders `netbox.env` and `netbox-postgres.env` under `/etc/netbox` at mode
  `0600`; the image reads all of its configuration from those variables.
- Creates the bind-mounted media, reports and scripts directories owned by the
  image's `netbox` account, plus the PostgreSQL and Valkey data directories.
- Provisions the read-only identity used by the NetBox MCP server (see below).

Database migrations, `collectstatic`, and superuser creation all run in the
image entrypoint on container start, so the role does not invoke them.
Housekeeping needs nothing either: NetBox registers it as a built-in system job
that the worker's scheduler runs daily.

## Networking

Only `netbox` publishes a port (`netbox_port` → Granian's 8080), and Granian
serves `/static` itself, so the deployment has no reverse proxy of its own;
Caddy terminates TLS and proxies straight to that port. PostgreSQL and both
Valkey instances are reachable only from the `netbox` Podman network and
therefore run without passwords — nothing outside that network can connect to
them.

## MCP identity

`tasks/mcp.yaml` provisions the account behind
[`scripts/netbox-mcp.sh`](../../../scripts/netbox-mcp.sh):

- a `mcp-readers` group holding one view-only `ObjectPermission`, covering every
  model in `netbox_mcp_permission_app_labels`,
- a `mcp-netbox` service user with no usable password, whose only grant comes
  from that group,
- the matching v2 API token with `write_enabled = False`.

Both steps run through `podman exec -i netbox … manage.py shell` reading a
rendered script on stdin; NetBox has no Ansible module, and stdin keeps the
token plaintext out of the process arguments. They are idempotent, correct
drift (a re-enabled write flag, extra actions, a directly attached permission),
and prune superseded tokens on the account.

### What makes it read-only

Three independent things, worth knowing before adjusting any of them:

- `write_enabled = False` on the token. `TokenPermissions` checks this ahead of
  any model permission, so every unsafe method is rejected regardless of what
  the account is otherwise granted. This is the load-bearing control.
- The `ObjectPermission` grants only `view`, and only on the listed apps.
- The account has no usable password, which closes
  `/api/users/tokens/provision/` — an unauthenticated endpoint that mints a
  token from a username and password.

Leaving `users` out of `netbox_mcp_permission_app_labels` does **not** hide the
account's own token: NetBox's built-in `DEFAULT_PERMISSIONS` grants every user
self-service `view`/`add`/`change`/`delete` on tokens constrained to `$user`
(likewise for bookmarks, notifications, and subscriptions in `extras`). So
`GET /api/users/tokens/` returns this account's own row. That row exposes
metadata only — `key` and `pepper_id`, with the plaintext serialized as `null`
and the secret unrecoverable from the HMAC — and the write flag is what stops
the account from minting itself a write-enabled token.

NetBox stores only an HMAC of a v2 token's secret half, so the value cannot be
read back after creation. The desired token is therefore generated outside
NetBox by `scripts/netbox-mcp-token.sh`, stored in `.env/secrets.sops.env`, and
passed in as `NETBOX_TOKEN` — the same value the MCP launcher uses. When
`NETBOX_TOKEN` is unset the group, permission, and user are still created and
the token step is skipped with a message. Like the other `manage.py` tasks in
this role, both steps are skipped under `--check`.

Rotation is the same flow as first setup: generate a new value, update the
encrypted env file, and re-run the play. A changed secret or a rotated
`API_TOKEN_PEPPERS` entry both surface as a digest mismatch, and the token row
is replaced.

## Variables

### Secrets (must be set in SOPS-encrypted files)

| Variable | Description |
|----------|-------------|
| `netbox_db_password` | PostgreSQL password for the `netbox` user |
| `netbox_secret_key` | Django secret key |
| `netbox_api_token_pepper` | Token hash pepper; required for the v2 API tokens NetBox issues by default |
| `netbox_superuser_password` | Password for the initial superuser |

### MCP identity variables (from the environment)

| Variable | Description |
|----------|-------------|
| `netbox_mcp_token` | v2 API token, read from `NETBOX_TOKEN`; empty skips token creation |

### Non-secret variables (in `defaults/main.yaml`)

| Variable | Default | Description |
|----------|---------|-------------|
| `netbox_version` | `4.7.0` | NetBox release half of the image tag |
| `netbox_docker_version` | `5.1.1` | netbox-docker release half of the image tag |
| `netbox_pg_image` | `docker.io/postgres:18-alpine` | PostgreSQL image; track the upstream compose file |
| `netbox_redis_image` | `docker.io/valkey/valkey:9.1-alpine` | Valkey image; track the upstream compose file |
| `netbox_domain` | `netbox.home.butaco.net` | `ALLOWED_HOSTS` and `CSRF_TRUSTED_ORIGINS` entry |
| `netbox_port` | `8080` | Published port Caddy proxies to |
| `netbox_base_dir` | `/opt/netbox` | Parent of the media, reports and scripts bind mounts |
| `netbox_pg_data_dir` | `/var/lib/netbox-postgresql` | PostgreSQL data directory |
| `netbox_redis_data_dir` | `/var/lib/netbox-redis` | Task-queue Valkey append-only file |
| `netbox_config_dir` | `/etc/netbox` | Directory holding both env files |
| `netbox_container_uid` / `netbox_container_gid` | `999` / `0` | Ownership the image expects on the bind mounts |
| `netbox_db_name` | `netbox` | PostgreSQL database name |
| `netbox_db_user` | `netbox` | PostgreSQL username |
| `netbox_superuser_name` | `admin` | Django superuser username |
| `netbox_superuser_email` | `admin@home.butaco.net` | Django superuser email |
| `netbox_mcp_enabled` | `true` | Provision the MCP identity |
| `netbox_mcp_username` | `mcp-netbox` | MCP service account username |
| `netbox_mcp_group` | `mcp-readers` | Group carrying the view-only permission |
| `netbox_mcp_permission_name` | `mcp-readers-view` | `ObjectPermission` name |
| `netbox_mcp_permission_app_labels` | `circuits`, `dcim`, `ipam`, `tenancy`, `virtualization`, `vpn`, `wireless` | Apps whose every model is readable; `users`, `core`, and `extras` are deliberately excluded, subject to the `DEFAULT_PERMISSIONS` caveat above |
| `netbox_mcp_permission_object_types` | `[]` | Extra individual object types as `app_label.model` |
| `netbox_mcp_token_description` | `NetBox MCP server (read-only)` | Description recorded on the token |

## Dependencies

- [`podman`](../podman/README.md), declared in `meta/main.yaml`.

## Usage

Run [playbooks/netbox.yaml](../../playbooks/netbox.yaml).
