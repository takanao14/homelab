# ADR-0023: Authenticate Ansible OpenBao operations via userpass login

- **Status:** Accepted
- **Date:** 2026-07-12
- **Related:** [ADR-0012](0012-openbao-eso-cluster-rebuild-registration.md)

## Context

OpenBao operational playbooks authenticated with a static admin token stored in
SOPS-encrypted group variables. The token was created during bootstrap with the
`admin` policy, but its lifetime was capped by OpenBao's system default maximum
TTL of 768 hours (32 days).

The token therefore expired silently even though it remained present in the
encrypted inventory. The next operational playbook then failed with HTTP 403
responses. This occurred on 2026-07-12 while running
`ops-openbao_seed_secrets.yaml`. The same failure during a k0s cluster rebuild
could block `ops-openbao_register_cluster.yaml` and prevent restoration of the
External Secrets Operator authentication path.

## Decision

Authenticate OpenBao operational playbooks at runtime through the `userpass`
auth method. Bootstrap creates or updates a dedicated `ansible-admin` user with
the `admin` policy. Each operational play obtains a token through userpass login
and stores it only as an in-memory Ansible fact for the duration of the play.
Existing tasks continue to consume the token through `BAO_TOKEN`.

Tokens issued to `ansible-admin` have a one-hour TTL. The long-lived password is
stored in SOPS-encrypted group variables. The OpenBao root token remains
reserved for bootstrap and recovery, including creation and password rotation
of the Ansible user.

## Alternatives considered

- **Keep issuing static admin tokens:** rejected because every token still
  expires after the system maximum TTL and requires recurring manual recovery.
- **Use a periodic token and renew it:** rejected because the token expires when
  no playbook renews it within the renewal window, preserving the same class of
  latent operational failure.
- **Use AppRole:** rejected because storing both the role ID and secret ID in the
  same SOPS-encrypted inventory provides no meaningful security improvement for
  this environment while adding lifecycle and implementation complexity.

## Consequences

- Operational access no longer depends on a stored token that silently expires.
- Tokens exposed to an Ansible process are short-lived and exist only during a
  playbook run.
- SOPS contains a long-lived Ansible password, which must be protected and
  rotated through the root-token bootstrap path if compromised.
- Bootstrap becomes a prerequisite for operational playbooks because it enables
  userpass and creates the dedicated user.
- The root token remains a sensitive recovery credential but is no longer used
  by routine OpenBao operations.
