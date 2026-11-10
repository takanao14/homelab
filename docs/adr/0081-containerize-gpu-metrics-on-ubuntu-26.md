# ADR-0081: Run GPU metrics in a container on Ubuntu 26.04

- **Status:** Accepted
- **Category:** Observability
- **Date:** 2026-10-29
- **Related:** [ADR-0077](0077-select-guest-os-by-role.md),
  [ADR-0079](0079-minimize-container-service-privileges.md)

## Context

Ubuntu 26.04 is the target for the next GPU worker rebuild. AMD supports its
ROCm stack on Ubuntu 26.04, but Device Metrics Exporter 1.5.3 publishes native
APT suites for Jammy and Noble, not Resolute. Reusing Noble packages would leave
the new guest dependent on a different Ubuntu release. The device plugin also
requires the exporter's GPU health service, so removing metrics alone is not a
valid migration path.

## Decision

Use AMD's official Device Metrics Exporter container on Ubuntu 26.04, managed by
rootful Podman and systemd. Keep the native package and services on Ubuntu 24.04.
Pin the container release and SHA-256 digest; use its bundled gpuagent and SMI
libraries rather than installing Noble exporter packages or building on the guest.

Follow AMD's [standalone container requirements](https://instinct.docs.amd.com/projects/device-metrics-exporter/en/latest/configuration/docker.html)
for privileged execution, GPU devices and a read-only `/sys` mount. Preserve
the existing metrics endpoint. Keep the exporter configuration host-managed with
`HealthService.Enable` enabled, and share `/var/lib/amd-metrics-exporter` with
the host so the existing device plugin can reach the health socket. Do not run
the native exporter or gpuagent alongside the container.

Stop the exporter before driver maintenance, as required by AMD's
[service management instructions](https://github.com/ROCm/device-metrics-exporter/blob/v1.5.3/docs/installation/docker.md).
Require GPU metrics, device-plugin health, inference and reboot recovery checks
before cutting over the production GPU worker.

## Consequences

- The exporter no longer blocks the OS migration on an unpublished APT suite.
  AMD documents Ubuntu 22.04 or later with a Docker-compatible runtime for
  its container deployment; RX 9060 XT passthrough requires local validation.
- Privileged device access is confined to the dedicated GPU guest, but still
  gives the exporter broad guest access. Reduce those privileges only after
  validating metrics and health with the pinned image.
- Container upgrades are independent of native exporter APT releases. Retain
  the release-specific configuration and health socket contract when updating.
