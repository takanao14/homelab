# ADR-0085: Simplify imagebuilder publication and process state

- **Status:** Accepted
- **Category:** Provisioning
- **Date:** 2026-11-02
- **Amends:** [ADR-0084](0084-on-demand-systemd-image-builder.md)
- **Related:** [ADR-0006](0006-custom-image-pipeline-monorepo-and-seaweedfs-s3.md),
  [`packer/README.md`](../../packer/README.md)

## Context

The builder hands images to a separate, operator-run Proxmox registration step
through fixed S3 object names and checksum sidecars. Keeping the previous pair
available after failed publication is unnecessary for this boundary. Backups,
automatic restoration and a global publication guard add recovery state without
being required by the consumer contract.

## Decision

Preserve the fixed-name image/checksum interface. Report publication success only
after both uploads and remote checksum verification succeed. Report failures
without restoring the previous pair or blocking later builds. Consumers remain
excluded until a verified publication succeeds. Retry publication using the same
retained, verified local artifacts, even if the existing remote pair is missing
or inconsistent. A read-only verifier can check the current pair independently.

Keep the existing systemd template service and single Go binary. Systemd owns
process lifetime, runtime limits and child termination; Go owns job admission,
business progress, artifact validation and durable results. Finalization must
not accept a worker success record when systemd reports process failure. Keep
the shared admission/shutdown lock, secret cleanup and boot recovery. Do not
replace these boundaries with shell traps or transient units.

Retain structured log storage and its pre-storage allowlist extraction. Moving
storage to journald requires a separate evaluation of remote access, retention
and cursor compatibility.

## Consequences

A failed publication may leave an unusable remote pair until republication.
Unrelated builds remain available; a successful build alone does not repair a
failed publication. Retry requires retained local artifacts and fresh credentials;
after retention expires, rebuild before publishing. Existing remote backups are
not used or deleted automatically. Proxmox registration remains a separate
operator action, with no automatic Terraform execution.
