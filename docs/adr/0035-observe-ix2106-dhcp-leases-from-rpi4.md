# ADR-0035: Observe IX2106 DHCP leases from rpi4

- **Status:** Accepted
- **Date:** 2026-08-16
- **Related:** [ADR-0001](0001-service-oriented-ansible-playbook-organization.md),
  [ADR-0002](0002-dhcp-outside-proxmox-cluster-nodes.md),
  [ADR-0003](0003-proxmox-host-log-collection-via-rsyslog-forwarding.md),
  [ADR-0025](0025-run-meshcentral-outside-managed-cluster.md),
  [`ansible/roles/node_exporter`](../../ansible/roles/node_exporter/)

## Context

The IX2106 at `192.168.10.1` is the authoritative DHCP server for the dynamic
LAN pool `192.168.10.30-127`. DNS telemetry identifies clients by IP address,
but an address can move between devices over time. Incident investigation needs
a time-bounded IP-to-device mapping without retaining full packet captures.

The Kea server on rpi4 has a different recovery role. ADR-0002 places it outside
the Proxmox and Kubernetes failure domains to serve the AMT network. Its lease
database therefore cannot be used as the source of truth for IX2106-managed LAN
leases. Co-locating an observer on rpi4 does not transfer DHCP authority from
the IX2106 to Kea.

The IX2106 does not expose its DHCP lease table through the SNMP MIB used here.
Its Web UI can download `show ip dhcp lease`-equivalent text, but that path
requires an administrator credential and relies on an undocumented Web
endpoint. Link Manager is visible to a Monitor user and can provide device
metadata, but it is not an authoritative lease table.

Read-only discovery on 2026-08-16 observed the following on the current device:

- SSH banner `NEC-IX2106-ms-10.9.34`.
- Password authentication only.
- RSA host key with a previously known and freshly observed matching SHA256
  fingerprint.
- `diffie-hellman-group-exchange-sha256`, AES-CTR and HMAC-SHA2 as the negotiated
  SSH primitives. Modern OpenSSH requires `ssh-rsa` to be enabled explicitly
  for this host key.
- A successful ICMP probe from the existing blackbox exporter on rpi4 to bgw1.

Subsequent validation with the dedicated Monitor account confirmed TCP/22 from
rpi4, a `%` operation prompt, a `%` global-configuration prompt after
`configure`, pager suppression with `terminal length 0`, and successful
`show version`, `show clock`, `show ip dhcp lease` and `show arp entry` commands.
The observed snapshot contained 24 lease rows and 41 ARP rows. No authorization
error or decoding replacement character occurred.

The IX2106 closes an otherwise complete interactive SSH session without sending
an SSH exit status, so OpenSSH reports 255 even after both `exit` commands. The
collector must use authenticated command boundaries, expected prompts and a
complete parse as its success criteria; process exit status alone is not a
reliable success signal for this device. The RSA MD5 fingerprint was then
confirmed through an independent Administrator/console management path and
matched the key used by the polling connection. An anonymized lease and ARP
fixture set was generated without writing the raw response to disk.

## Decision

Run `dhcp-lease-observer` on rpi4 as a systemd oneshot service invoked by a
timer. Poll the IX2106 over SSH with a dedicated Monitor user and a fixed command
sequence. The DHCP lease and ARP show commands run in global configuration mode
on IX software 10.9, even for Monitor users: enter with `configure`, set the
session pager with `terminal length 0`, run only the required show commands, and
exit immediately. Pin the independently verified host key fingerprint and
enable only the legacy algorithms the observed firmware requires. Do not
support arbitrary commands through configuration or command-line arguments.

Use Python 3 only for a manually invoked proof of concept that records
anonymized parser fixtures. Implement the production collector in Go and deploy
it as a statically linked linux/arm64 binary; do not install a Go toolchain or a
Python application environment on rpi4 for the production service.

Keep the collector source in the public MIT-licensed
[`dhcp-lease-observer`](https://github.com/takanao14/dhcp-lease-observer)
repository, not under this homelab IaC repository. That repository owns the Go
module, source adapters, anonymized fixtures, tests, release workflow and
versioned binaries.

This repository owns only the Ansible deployment, SOPS-encrypted site secrets,
systemd configuration, pinned release version and SHA256, Prometheus alerts and
Grafana dashboards.

Publish versioned linux/arm64 and linux/amd64 archives with a checksum manifest
from the collector repository's CI. The homelab inventory pins an immutable tag
and the expected archive SHA256; Ansible downloads that exact asset and rejects
a checksum mismatch. Do not consume `latest`, build source during an Ansible
run, commit binaries here, or install the Go toolchain on rpi4. The collector
repository is public only while automated tests establish that fixtures and
release metadata contain no site-specific addresses or identifiers. Raw device
output never becomes a repository or release input.

Publish current state and collector health through the node_exporter textfile
collector already scraped on rpi4. Emit lease changes as JSON Lines to journald,
then reuse the existing Vector-to-Loki path. Preserve a last-good snapshot on
collection or parser failure, and never infer that every lease disappeared from
a partial response.

Run the service as a dedicated non-root local user. Supply the IX2106 password
and device-identity HMAC key through systemd credentials sourced from
SOPS-encrypted inventory. Pin the target host, port and commands. Treat Monitor
as reduced privilege, not as a strict read-only command allowlist. The IX2106
SSH access list applies to the server rather than to an individual local user;
review it to include rpi4 and existing management sources without claiming a
per-user source restriction the platform does not provide.

Do not use the Web administrator download endpoint. Consider Monitor-level Link
Manager data later only as optional hostname, Vendor ID, port and last-seen
enrichment; it must not replace the CLI lease source.

## Alternatives considered

- **Poll the Web UI DHCP download.** Rejected. It returns the same unstructured
  CLI text while requiring a broadly privileged administrator credential and
  an undocumented session/API contract.
- **Use Link Manager as the lease source.** Rejected. It is a discovered device
  inventory and can contain stale, static or indirectly observed devices; it
  does not carry authoritative lease state and lifetime semantics.
- **Read Kea leases on rpi4.** Rejected for the LAN pool. Kea observes only the
  scopes it serves and cannot report leases owned by the IX2106.
- **Run a persistent HTTP exporter.** Rejected. The existing node_exporter
  textfile and Vector pipelines cover the required metrics and events without a
  new listener or scrape target.
- **Keep the production collector in Python.** Rejected. A Go single binary
  provides a smaller runtime and dependency surface on the recovery host while
  retaining practical SSH and testing support.
- **Keep the Go module under `tools/` in the homelab repository.** Rejected. It
  couples application release cadence and language dependencies to IaC changes,
  leaves no clean immutable artifact boundary, and makes reuse or a future
  language migration unnecessarily site-specific.
- **Build the binary on the Ansible controller.** Rejected for production. It
  makes a deployment depend on an unrecorded local toolchain and an ignored
  workspace artifact. CI-built versioned archives plus a checksum pinned in the
  homelab repository make the deployed input explicit.
- **Implement the first version in Rust.** Deferred. Its runtime properties do
  not outweigh the additional SSH/PTY implementation and maintenance cost for
  this small periodic collector. Fixtures and schemas remain language-neutral
  so the choice can be revisited.

## Consequences

- rpi4 gains another lightweight recovery-adjacent workload and an outbound SSH
  dependency on bgw1. Its failure stops fresh attribution but does not affect
  DHCP service.
- The observer credential has less authority than a Web administrator but is
  not assumed to be cryptographically constrained to show commands. Source ACLs,
  fixed commands and systemd sandboxing are required defense in depth.
- Firmware changes to the SSH banner, algorithms, prompt or CLI table format
  fail closed and leave the last-good snapshot in place until fixtures and the
  parser are updated.
- A transport-level missing exit status is not itself a collection failure on
  this IX2106 firmware. Success still requires every fixed command boundary,
  expected Monitor prompts, no authorization error and a complete validated
  lease response.
- Global configuration mode is exclusive on the IX2106. A concurrent operator
  session can return `CONFIG process is occupied`; the observer treats this as
  an expected collection failure, exits without using `svintr-config`, and
  retries on the next timer run.
- The node_exporter textfile directory needs a shared writer group managed by
  the node_exporter role. Only explicitly selected service users join it.
- Device identifiers remain stable by using an HMAC of the normalized MAC
  address. Rotating the HMAC key deliberately breaks identity continuity.
- Web Link Manager, Kea, ntopng and SPAN capture remain separate decision gates;
  none are part of the initial implementation.
- Collector source and release changes occur in a separate repository and are
  deployed here only after deliberately updating the pinned version and SHA256.
  The repositories do not share commits or require synchronized default
  branches.
- Publishing the collector repository requires fixture redaction checks. Raw
  captures, credentials, host key fingerprints and site-specific inventory
  remain private and never become release inputs.

## Acceptance prerequisites

The device-access prerequisites below were verified on 2026-08-16. Repository
and release prerequisites remain gates for accepting the implementation rather
than for beginning the Go work.

- A dedicated Monitor user can enter global configuration mode, set the
  session-only terminal length, and run `show ip dhcp lease` and `show arp entry`
  without changing persistent configuration.
- The IX2106 SSH access list is reviewed before adding rpi4, preserving every
  existing management source. No per-user source ACL is assumed.
- The host key fingerprint is verified through the IX2106 console or another
  management path independent of the polling connection.
- Anonymized fixtures cover normal, empty, paged, truncated and failure output.
- The collector repository publishes a tested linux/arm64 release archive, and
  the homelab role verifies a SHA256 pinned independently in this repository.
- The repository's Kea configuration is documented explicitly as the AMT scope,
  while the IX2106 remains authoritative for the LAN dynamic pool.
