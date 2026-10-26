# podman Role

Installs Podman and its rootless prerequisites (`passt`, `uidmap`, `slirp4netns`,
`fuse-overlayfs`) from the Ubuntu `universe` repository, and ensures the
root system Quadlet directory (`/etc/containers/systemd`) exists.

Podman has no persistent daemon, so there is no service to enable/start
in the default rootful entrypoint. Consuming roles (e.g. `lldap`) drop `*.container` Quadlet unit
files into `podman_quadlet_dir`, notify `Reload systemd`, and then manage
the generated `<name>.service` unit with the normal `ansible.builtin.service`
module. These rootful consumers do not use `systemctl --user` or lingering;
the opt-in rootless entrypoint below starts a persistent user manager.

## Variables

| Variable | Default | Description |
|----------|---------|--------------|
| `podman_packages` | `[podman, passt, uidmap, slirp4netns, fuse-overlayfs]` | apt packages to install |
| `podman_quadlet_dir` | `/etc/containers/systemd` | Root system Quadlet unit directory |

## Usage

Declare as a role dependency (see `ansible/roles/docker`'s equivalent
pattern) or list directly in a playbook before the consuming role.

## Opt-in rootless services

The `rootless` task entrypoint prepares a dedicated local account, fixed UID/GID,
non-overlapping subordinate IDs, a private home, and a lingering user manager.
Call it with `import_role` and `podman_rootless_user`, `podman_rootless_uid`, and
`podman_rootless_subid_start`. Existing conflicting allocations fail rather than
being remapped. The default subordinate range size is 65536.

OpenClaw and Hermes retain system units with `User=`, `Delegate=yes`, and access
to that account's user bus. Podman runs rootless while systemd orders the service
after both its user manager and root-owned firewall. Firewall rules are replaced
atomically and retained when the firewall unit stops or fails. Runtime credential
parents remain root-owned; only the service account can read their `0600` files.
Rootless units use `--userns=host`, which reuses Podman's rootless user namespace,
not the host's initial namespace. Container root maps to the service account;
other IDs map to subordinate IDs without remapping image layers for `keep-id`.

Rootless initialization is explicit and requires a maintenance window. For OpenClaw use
`openclaw`, service `openclaw`, account `openclaw-svc` (UID/GID 61001,
subordinate start 2000000); for Hermes use `hermes`, service `hermes-dashboard`,
account `hermes-svc` (61002, start 2065536). The roles default to rootful operation;
the homelab inventory enables rootless. Keep the application version unchanged.
These two agents recover through an empty-state rebuild from Ansible/SOPS;
backups are not required, and sessions, learned state and device pairings are lost.

1. Prepare the account without deploying the service, for example from `ansible/`:
   `ansible-playbook playbooks/services/openclaw.yaml --tags rootless_prepare -e '{"openclaw_rootless_enabled": true}'`.
2. Stop and disable the existing service. Verify the old rootful container has
   disappeared (`podman container exists` must return 1), and retain the existing
   state, credentials, and unit backup with restricted access.
3. For a fresh build, leave the new `-rootless` state directory absent; Ansible
   creates it empty with the mapped application UID/GID. Old sessions, memory
   and device pairings are not imported. Keep the original state for rollback.
   To retain application data instead, copy the stopped `/var/lib/openclaw` to
   `/var/lib/openclaw-rootless` (Hermes: `/var/lib/hermes-rootless`). Preserve contents, modes and
   symlinks without following them. Copy regular files to independent inodes;
   never hard-link the copy to live state. Use host-root `chown -hRP` **only on
   the copy** to assign the mapped application UID/GID: subordinate start +
   container ID - 1.
   Defaults are 2000999:2000999 for OpenClaw (container 1000:1000) and
   2075535:2075535 for Hermes (container 10000:10000). Verify the actual UID/GID
   maps with the service account's `podman unshare` before changing ownership.
   The original state must remain untouched for rollback. Confirm the copy is
   complete before setting the readiness flag. No copy or recursive chown is
   needed for a fresh build.
4. Persist `openclaw_rootless_enabled: true` and
   `openclaw_rootless_state_prepared: true` in the applicable inventory (or the
   equivalent `hermes_` variables); the readiness flag confirms the choice of
   fresh or restored state. Config defaults move to `/etc/openclaw-rootless`
   or `/etc/hermes-rootless`; Ansible recreates credentials from SOPS. Existing
   rootful state/config paths are rejected. Run `ansible-playbook --check` first,
   then the service playbook. The role refuses to proceed while a rootful
   container with the service name still exists.
5. Verify authentication, WebSocket, SSH recovery, inference/MCP, state retention,
   Caddy-only access, logs, and a controlled reboot before accepting migration.
   `User=` in a system unit requires a rootless runtime smoke test with the
   deployed Podman/systemd versions before adoption; successful unit parsing
   alone does not prove cgroup or user-session compatibility.

For rollback, stop the rootless service and confirm its container has exited,
restore the rootful inventory settings and deploy against the untouched original
state. Decide explicitly how to recover writes made after cutover before rolling
back; never overwrite either state copy automatically. Keep old Podman storage
and backups until rollback and retention requirements are satisfied.
