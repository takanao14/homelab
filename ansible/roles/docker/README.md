# docker Role

Installs Docker CE (`docker-ce`, `docker-ce-cli`, `containerd.io`,
`docker-compose-plugin`) from the upstream Docker apt repository on
Debian-based systems, and ensures the service is running.

Shared by roles that need a container runtime (`forgejo_runner`,
`authentik`) instead of each duplicating the apt repo setup. Consumers add
their own user to the `docker` group and manage their own containers/compose
files after this role runs.

## Variables

### Non-secret variables (in `defaults/main.yaml`)

| Variable | Default | Description |
|----------|---------|-------------|
| `docker_packages` | `[docker-ce, docker-ce-cli, containerd.io, docker-compose-plugin]` | apt packages to install |
| `docker_daemon_config_path` | `/etc/docker/daemon.json` | Path for the daemon config |
| `docker_daemon_config` | `{}` | Dict merged into `daemon.json`; skipped when empty |

## Usage

Declare as a role dependency (see `forgejo_runner/meta/main.yaml` for an
example) or list directly in a playbook before the consuming role.
