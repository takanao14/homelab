# ADR-0050: Render every Authentik blueprint from one set of variables

- **Status:** Accepted
- **Date:** 2026-09-18

## Context

The Authentik LDAP Outpost and its SSSD clients share one interface: the LDAPS
service name and port, the provider Base DN, the lookup bind account, and the
group names that authorize login and sudo. Both sides wrote every one of those
values as a literal, and `roles/authentik` wrote several of them twice because
its blueprints were static files and could not read the role's own defaults.
The certificate CN lived in `authentik_ldap_cert_cn` while the name it must
match lived in the blueprint's `tls_server_name`; the Outpost names and the
browser-facing proxy URL were duplicated the same way.

Nothing compared the two sides. Changing one alone produces no plan diff, no
lint error, and no failed task; it produces hosts that cannot resolve users or
cannot validate the LDAPS certificate, which surfaces only at the next login.

Group names drift differently. A policy binding that names a group no longer in
`groups.yaml` does fail, because `!Find` cannot resolve it — but only inside
Authentik. The role never read the resulting blueprint status, and Authentik
keeps the previous status until the worker finishes, so the run stayed green.

## Decision

Declare the cross-role contract once as `identity_*` variables in
`inventories/homelab/group_vars/all.yaml`, and the group names once as
`authentik_groups` in the Authentik role defaults. Render every blueprint from
`templates/blueprints/`; `files/blueprints/` no longer exists.

`groups.yaml` creates exactly the `authentik_groups` entries, and every binding
addresses them by key, so naming a group that is not defined raises an undefined
attribute while rendering, before anything is deployed. `lab-ldap-service` stays
out of the dictionary because it carries an RBAC role and is created by the LDAP
blueprint rather than by `groups.yaml`.

Apply all blueprints explicitly in `authentik_blueprint_paths` order rather than
waiting for Authentik's periodic discovery, then fail the run when any of them
does not report a successful apply.

## Consequences

- Renaming the LDAPS service, moving the Base DN, or renaming a group is a
  single edit that every consumer follows.
- A binding to an undefined group fails at render time on the control node. A
  blueprint that fails for any other reason fails the run instead of leaving a
  silently broken Outpost.
- The status check reads the state Authentik recorded. Authentik keeps the
  previous status until the worker finishes, so a failure introduced by the
  current run can be reported by the next one rather than immediately.
- Editing `ldap.yaml` or `proxy.yaml` now takes effect during the run. Before,
  only `users.yaml` was applied explicitly and the rest waited for discovery;
  `groups.yaml` had not been reapplied since the initial deployment.
- Role defaults reference inventory variables, so neither role runs against an
  inventory that omits them. That failure is immediate and explicit, which is
  the intent; these roles are not published for reuse elsewhere.
- The certificate PEM continues to travel from the Authentik role into
  `roles/sssd/files/` through `fetch`; only the names around it are unified.
