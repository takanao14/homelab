# podman Role

Installs Podman and its rootless prerequisites (`uidmap`, `slirp4netns`,
`fuse-overlayfs`) from the Ubuntu `universe` repository, and ensures the
root system Quadlet directory (`/etc/containers/systemd`) exists.

Podman has no persistent daemon, so there is no service to enable/start
here. Consuming roles (e.g. `authentik`) drop `*.container` Quadlet unit
files into `podman_quadlet_dir`, notify `Reload systemd`, and then manage
the generated `<name>.service` unit with the normal `ansible.builtin.service`
module — same as any other systemd-managed service in this repo (no
`systemctl --user` / lingering involved; units run as root system services).

## Variables

| Variable | Default | Description |
|----------|---------|--------------|
| `podman_packages` | `[podman, uidmap, slirp4netns, fuse-overlayfs]` | apt packages to install |
| `podman_quadlet_dir` | `/etc/containers/systemd` | Root system Quadlet unit directory |

## Usage

Declare as a role dependency (see `ansible/roles/docker`'s equivalent
pattern) or list directly in a playbook before the consuming role.
