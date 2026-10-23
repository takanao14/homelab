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

Keep rootless disabled by default until state migration and runtime validation
are complete. Require separate state and credential paths; never recursively
change the ownership of the live rootful state. Preserve container identities,
shutdown budgets, proxy restrictions and the shared MCP credential source.
Host systemd hardening must not prevent Podman's subordinate UID helpers from
acquiring the privileges needed to create its user namespace.

Apply `no-new-privileges` to the other Podman services. Drop all capabilities
from the already non-root NetBox web and worker containers; retain the startup
capabilities of database and directory images that change ownership or users.
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
  or changing their data ownership. Enabling it still requires a stopped-state
  copy, UID mapping validation, startup and functional checks, and rollback.
- App credentials remain accessible to the app and its dedicated host account;
  rootless execution does not reduce the permissions granted by those tokens.
- System unit names remain stable for journald and service operations. Podman
  image operations and Hermes CLI access must use the selected host account.
- The rootful Docker socket still grants root-equivalent host control. Runner
  process restrictions and job configuration do not remove that authority.
