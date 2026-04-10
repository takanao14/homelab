# forgejo Role

Installs and configures [Forgejo](https://forgejo.org/) self-hosted Git service on Debian-based systems.

## Functionality

- Creates a dedicated system user/group (`git`).
- Downloads the Forgejo binary from Codeberg releases.
- Deploys `/etc/forgejo/app.ini` from a Jinja2 template.
- Deploys and enables a systemd unit.
- Ensures the service is started and enabled.

## Variables

### Secrets (must be set in inventory vars or SOPS-encrypted files)

| Variable | Description |
|----------|-------------|
| `forgejo_secret_key` | Secret key for CSRF/encryption |
| `forgejo_internal_token` | Internal API token |
| `forgejo_jwt_secret` | JWT signing secret |
| `forgejo_lfs_jwt_secret` | LFS JWT signing secret |

### Non-secret variables (in `defaults/main.yaml`)

| Variable | Default | Description |
|----------|---------|-------------|
| `forgejo_version` | `14.0.3` | Forgejo version to install |
| `forgejo_user` | `git` | System user |
| `forgejo_group` | `git` | System group |
| `forgejo_home` | `/var/lib/forgejo` | Home and data directory |
| `forgejo_config_dir` | `/etc/forgejo` | Config directory |
| `forgejo_binary` | `/usr/local/bin/forgejo` | Binary path |
| `forgejo_domain` | `forgejo.home.butaco.net` | Public domain name |
| `forgejo_http_port` | `80` | HTTP listen port |
| `forgejo_ssh_port` | `2222` | SSH listen port |
| `forgejo_db_type` | `sqlite3` | Database backend |
| `forgejo_db_path` | `{{ forgejo_home }}/data/forgejo.db` | SQLite database path |

## Dependencies

None.

## Usage

```yaml
- name: Setup Forgejo
  hosts: forgejo
  roles:
    - role: forgejo
```
