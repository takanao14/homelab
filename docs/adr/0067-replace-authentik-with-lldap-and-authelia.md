# ADR-0067: Replace Authentik with LLDAP and Authelia

- **Status:** Proposed
- **Date:** 2026-10-08
- **Review trigger:** SSSD and forward-auth acceptance checks pass against LLDAP and Authelia
- **Related:** [ADR-0010](0010-sandbox-argocd-uses-http-only-gitops-bootstrap.md), [ADR-0050](0050-single-definition-for-the-authentik-ldap-contract.md), [ADR-0058](0058-select-authentik-features-and-update-them-individually.md), [ADR-0060](0060-mcp-relies-on-bearer-authentication.md)

## Context

Authentik serves two consumers: SSSD through the LDAP Outpost, and Headlamp and
GPU Switch through Proxy Outpost forward auth. OIDC and MFA remain unused. Those
two functions require PostgreSQL, a server, a worker, two Outposts, rendered
blueprints, API-retrieved Outpost tokens, and sequential release-series
upgrades across about 30 releases a year.

Its LDAP view also departs from RFC2307bis. Login names are in `cn`, disabled
users keep their entries and groups behind `ak-active`, the Outpost adds a
second revocation cache, and group headers are pipe-separated while Headlamp
splits on commas. `roles/sssd` carries a workaround for each.

Other web UIs use unrelated mechanisms: local administrators, bearer tokens,
source CIDR policies, or no authentication.

## Decision

Use LLDAP as the only user and group directory and Authelia as the web
authentication service. Run both outside the clusters on one Podman Quadlet VM,
as Authentik runs now, and remove Authentik after cutover.

- **Linux:** SSSD binds to LLDAP over LDAPS with `uid` login names and LLDAP's
  RFC2307bis schema. `uidNumber`, `gidNumber`, `homeDirectory`, `unixShell`, and
  `sshPublicKey` are LLDAP custom attributes that Ansible populates. Each user's
  primary group is a personal group whose `gidNumber` equals the user's
  `uidNumber`, matching Authentik's virtual groups. Revocation removes group membership or deletes the user.
- **Migration:** No Authentik data is migrated. Ansible declares users, groups,
  numeric IDs, and SSH keys and builds LLDAP from scratch, as it rebuilds
  Authentik today.
- **Contract:** ADR-0050's `identity_*` variables remain the single definition
  shared by the directory and SSSD roles. They name only standard attributes so
  another RFC2307bis directory can replace LLDAP without changing clients.
- **Web, `*.home` and `*.prd`:** Authelia authenticates against LLDAP with one
  session cookie domain, `butaco.net`, and its portal at
  `https://auth.home.butaco.net`. Applications without suitable native login
  use forward auth: Envoy Gateway `SecurityPolicy` extAuth in prd and Caddy
  `forward_auth` in the home zone. Applications with native OIDC use Authelia as
  their OIDC provider and keep a local administrator for break-glass access.
- **Web, `*.sandbox`:** Authelia requires HTTPS, and ADR-0010 keeps sandbox
  HTTP-only. Sandbox UIs therefore leave single sign-on and use Envoy Gateway
  source CIDR policies.
- **MFA:** Authelia policies use `one_factor`. MFA remains deferred.
- **Unchanged:** Machine APIs keep bearer tokens (ADR-0060). Kubernetes API
  server OIDC remains deferred. LLDAP, Authelia, network appliances, and TrueNAS
  keep local authentication only.

## Alternatives considered

Activity figures are for the 12 months before 2026-10-08.

- **Keep Authentik.** It is the most active candidate (33 releases), but the
  operating cost and LDAP divergence above are the reason for this change.
  *Rejected.*
- **Authentik fed by an LLDAP source.** This fixes the Linux schema but keeps the
  full Authentik stack for two web applications. *Rejected.*
- **Kanidm.** It is active (22 releases) and combines directory and OIDC, but its
  LDAP interface is read-only and the supported Linux path is `kanidm-unixd`
  rather than SSSD over LDAP. *Rejected.*
- **Keycloak or Zitadel.** Both are well maintained OIDC providers but heavier
  than Authentik and still need a separate directory for SSSD. *Rejected.*
- **Pocket ID with tinyauth.** Both are active but young and largely
  single-maintainer. Pocket ID is passkey-only. *Rejected.*
- **GLAuth, 389 Directory Server, or FreeIPA as the directory.** GLAuth is less
  active than LLDAP and has no management UI; 389-ds has no management layer
  without FreeIPA; FreeIPA is justified only by Kerberos, HBAC, central sudo, or
  host enrollment needs. *Rejected.*

## Consequences

- LLDAP is effectively single-maintainer and released once in the year before
  this decision. LDAPS stays reachable only from the LAN, Renovate tracks its
  image, and the standard-attribute contract keeps replacement local to the
  directory role.
- Every account receives a new password at cutover. Numeric IDs may differ from
  Authentik's, so existing LDAP users' home directories on SSSD hosts are not
  carried over.
- Revocation is bounded by SSSD's `entry_cache_timeout` alone. LLDAP has no
  disabled state; offboarding is group removal or deletion.
- Forward-auth identity headers change from `X-authentik-*` to Authelia's
  `Remote-*` headers, and groups are comma-separated as Headlamp expects.
- The sandbox Headlamp uses a cluster-admin ServiceAccount token for every
  request. Its CIDR policy must be shown to deny a disallowed client; the shared
  Gateway SNAT described in ADR-0060 can make it ineffective. If it cannot be
  enforced, sandbox Headlamp returns to token login.
- ADR-0058 no longer applies once Authentik is removed.
- LLDAP and Authelia are each a single instance. Their failure blocks new Linux
  lookups beyond the SSSD cache and new web logins; break-glass accounts remain
  the recovery path.
