# ADR-0079: Minimize container service privileges without changing engines

- **Status:** Accepted
- **Category:** Configuration & operations
- **Date:** 2026-10-23
- **Related:** [ADR-0025](0025-run-meshcentral-outside-managed-cluster.md), [ADR-0046](0046-netbox-containers-on-a-node3-vm.md), [ADR-0063](0063-openclaw-on-dedicated-vm-with-lemonade.md), [ADR-0066](0066-hermes-dashboard-through-caddy.md), [ADR-0067](0067-replace-authentik-with-lldap-and-authelia.md)

## Context

Service containers run under rootful Podman; Forgejo Runner uses rootful Docker.
Application UIDs vary independently of the engine's privileges. OpenClaw runs as
UID 1000, while Hermes has both root and UID 10000 processes. Dedicated VMs limit
the failure domain but do not replace host privilege separation.

OpenClaw and Hermes require a root-managed firewall before publishing their
ports. Moving their services to a user systemd manager would split this ordering
across managers. Existing state and root-owned credential files also cannot be
reused under a different UID mapping without a controlled migration.

## Decision

Add opt-in rootless Podman support for OpenClaw and Hermes using dedicated local
accounts. Keep application units in the system manager with `User=` and `Group=`
so their root-managed firewall dependencies and existing journal unit names stay
intact. Provision subordinate IDs and a lingering user manager for Podman's
systemd cgroup backend, and pass its runtime directory and D-Bus address.
This uses plain systemd services, not system Quadlets with a substituted user.

Keep rootless disabled by default until state initialization and runtime validation
are complete. Require separate state and credential paths; never recursively
change the ownership of the live rootful state. Preserve container identities,
shutdown budgets, proxy restrictions and the shared MCP credential source.
Host systemd hardening must not prevent Podman's subordinate UID helpers from
acquiring the privileges needed to create its user namespace.
Use Podman's standard rootless UID/GID mapping and assign application
state to its subordinate IDs. Avoid `keep-id`, whose first use with native
overlay requires image-layer ownership conversion. Container application IDs
remain unchanged; runtime credential files belong to the dedicated account.
An explicitly requested fresh build starts with empty state created by Ansible
and settings recreated from Ansible/SOPS; retain the old rootful state for rollback.
Recover OpenClaw and Hermes by rebuilding from Ansible/SOPS with empty state,
without requiring backups. Sessions, learned state and device pairings are
disposable; this amends ADR-0064's requirement to back up Hermes state.

Apply `no-new-privileges` to the other Podman services except NetBox's PostgreSQL
container. Use LLDAP's official v0.6.3 non-root image with explicit UID/GID 1000,
Ansible-managed data directory ownership, and no capabilities. Keep its bootstrap
credentials root-only and run bootstrap with explicit container UID 0.
Run Authelia as a fixed non-root container UID/GID without capabilities. Bypass
its root entrypoint initialization through the official direct execution path.
Mount Ansible-managed configuration and secrets read-only, and keep SQLite and
the notifier in a separate writable data directory. Require an explicit stopped
migration for existing state; rollback must preserve writes made after cutover.
Confine MeshCentral with a dedicated AppArmor profile based on the container
default. Permit crun's TERM/KILL and same-context signals/ptrace read for the
stacked crun/container label; do not change Kea or the OS-wide default profile.
The PostgreSQL `18-alpine` image fails startup with this restriction on netbox1.
Direct UID/GID 70 startup with no capabilities bypasses `gosu` but still stalls
during initialization; the same-image control without the restriction starts and
serves SQL. The cause is unresolved, so retain its existing startup permissions.
The pinned Valkey image passed with the restriction.
Drop all capabilities from the already non-root NetBox web and worker containers; retain the startup
capabilities of database images that change ownership or users.
Keep their current rootful deployment until service-specific migration and
recovery validation is complete. ADR-0046's NetBox runtime decision still applies.

Keep Forgejo Runner on Docker. Make its configuration root-managed and restrict
the runner process's capabilities, privilege escalation and file creation modes.
Expose job network and Docker socket automount policy explicitly. The existing
host network and socket remain defaults until actual workflows are validated.
Rootless Docker remains a separate compatibility evaluation; changing engines is
not a prerequisite for reducing privileges.

## Consequences

- Rootless execution is available without silently switching existing services
  or changing their data ownership. Enabling it requires explicit fresh state or
  a stopped-state copy, UID mapping validation, functional checks, and rollback.
- App credentials remain accessible to the app and its dedicated host account;
  rootless execution does not reduce the permissions granted by those tokens.
- System unit names remain stable for journald and service operations. Podman
  image operations and Hermes CLI access must use the selected host account.
- The rootful Docker socket still grants root-equivalent host control. Runner
  process restrictions and job configuration do not remove that authority.
