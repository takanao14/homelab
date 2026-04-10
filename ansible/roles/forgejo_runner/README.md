# forgejo_runner Role

Installs and configures a [Forgejo Actions Runner](https://code.forgejo.org/forgejo/runner) on Debian-based systems. Uses Podman for container execution.

## Functionality

- Creates a dedicated system user/group (`runner`).
- Installs dependencies: `git`, `podman`, `uidmap`, `slirp4netns`.
- Configures rootless Podman (subuid/subgid, lingering, user socket).
- Downloads the forgejo-runner binary from Forgejo releases.
- Registers the runner with the Forgejo instance (skipped if already registered).
- Deploys and enables a systemd unit.

## Variables

### Secrets (must be set in inventory vars or SOPS-encrypted files)

| Variable | Description |
|----------|-------------|
| `forgejo_admin_token` | Forgejo admin API token (used to fetch registration token) |

### Non-secret variables (in `defaults/main.yaml`)

| Variable | Default | Description |
|----------|---------|-------------|
| `forgejo_runner_version` | `12.8.0` | Runner version to install |
| `forgejo_runner_user` | `runner` | System user |
| `forgejo_runner_group` | `runner` | System group |
| `forgejo_runner_home` | `/var/lib/forgejo-runner` | Home directory |
| `forgejo_runner_binary` | `/usr/local/bin/forgejo-runner` | Binary path |
| `forgejo_runner_config` | `/etc/forgejo-runner/config.yml` | Config file path |
| `forgejo_runner_url` | `http://forgejo.home.butaco.net` | Forgejo instance URL |
| `forgejo_runner_name` | `{{ inventory_hostname }}` | Runner display name |
| `forgejo_runner_labels` | `self-hosted`, `ubuntu-24.04` | Runner labels (docker image mappings) |

## Dependencies

None.

## Usage

```yaml
- name: Setup Forgejo Runner
  hosts: forgejo_runner
  roles:
    - role: forgejo_runner
```

## Notes

- Runner registration is skipped if `{{ forgejo_runner_home }}/.runner` already exists.
- Podman socket is enabled via `systemctl --user -M runner@` (user systemd instance via machinectl).
