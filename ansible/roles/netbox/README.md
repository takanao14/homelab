# netbox Role

Deploys [NetBox](https://github.com/netbox-community/netbox) IPAM/DCIM on Debian-based systems using PostgreSQL, Redis, gunicorn, and nginx.

## Functionality

- Installs system dependencies (PostgreSQL, Redis, nginx, Python build tools).
- Creates a dedicated `netbox` PostgreSQL database and user with full privileges.
- Creates a `netbox` system user and group.
- Downloads and extracts NetBox from GitHub releases to `/opt/netbox-<version>` and symlinks to `netbox_home`.
- Creates a Python virtualenv and installs requirements including gunicorn.
- Deploys `configuration.py` from a Jinja2 template.
- Runs database migrations and collects static files.
- Creates the superuser if not already present.
- Deploys `gunicorn.py` config and systemd units for `netbox` and `netbox-rq` services.
- Deploys and enables the nginx virtual host config; removes the default site.
- Provisions the read-only identity used by the NetBox MCP server (see below).

## MCP identity

`tasks/mcp.yaml` provisions the account behind
[`scripts/netbox-mcp.sh`](../../../scripts/netbox-mcp.sh):

- a `mcp-readers` group holding one view-only `ObjectPermission`, covering every
  model in `netbox_mcp_permission_app_labels`,
- a `mcp-netbox` service user with no usable password, whose only grant comes
  from that group,
- the matching v2 API token with `write_enabled = False`.

Both steps run through `manage.py shell` reading a rendered script on stdin;
NetBox has no Ansible module, and stdin keeps the token plaintext out of the
process arguments. They are idempotent, correct drift (a re-enabled write flag,
extra actions, a directly attached permission), and prune superseded tokens on
the account.

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
| `netbox_version` | `4.6.0` | NetBox version to install |
| `netbox_user` | `netbox` | System user |
| `netbox_group` | `netbox` | System group |
| `netbox_home` | `/opt/netbox` | Symlink path to the active NetBox installation |
| `netbox_venv` | `/opt/netbox/venv` | Python virtualenv path |
| `netbox_domain` | `netbox-ui.home.butaco.net` | Domain name for the nginx virtual host |
| `netbox_port` | `8080` | gunicorn listen port |
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

- `community.postgresql` Ansible collection (`community.postgresql.postgresql_db`, etc.).

## Usage

```yaml
- name: Deploy NetBox
  hosts: netbox
  roles:
    - netbox
```
