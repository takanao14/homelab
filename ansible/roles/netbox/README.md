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

## Variables

### Secrets (must be set in SOPS-encrypted files)

| Variable | Description |
|----------|-------------|
| `netbox_db_password` | PostgreSQL password for the `netbox` user |
| `netbox_secret_key` | Django secret key |
| `netbox_api_token_pepper` | Token hash pepper (optional, improves token security) |
| `netbox_superuser_password` | Password for the initial superuser |

### Non-secret variables (in `defaults/main.yaml`)

| Variable | Default | Description |
|----------|---------|-------------|
| `netbox_version` | `4.5.9` | NetBox version to install |
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

## Dependencies

- `community.postgresql` Ansible collection (`community.postgresql.postgresql_db`, etc.).

## Usage

```yaml
- name: Deploy NetBox
  hosts: netbox
  roles:
    - netbox
```
