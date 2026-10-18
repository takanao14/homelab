# ADR-0075: Use Debian for DNS LXC guests

- **Status:** Accepted
- **Category:** Network & DNS
- **Date:** 2026-10-18
- **Related:** [ADR-0030](0030-in-house-full-service-resolver.md), [ADR-0051](0051-forward-recursive-cache-misses-over-dot.md)

## Context

DNS runs in seven Ubuntu LXC guests. Fresh-build comparisons found fewer
packages, lower disk usage, and lower package-upgrade memory peaks on Debian,
while retaining APT, systemd, and upstream package channels for dnsdist,
PowerDNS Authoritative, and Knot Resolver 6. DNS response times were comparable;
total cgroup memory did not decrease because file-cache usage was higher.

## Decision

Adopt Debian 13 for DNS guests after fresh-build validation. Recreate guests
through Terragrunt and configure them with shared Ansible roles, preserving
addresses, service boundaries, and resource allocations. Keep per-host OS
selection and the existing Terraform state layout for sequential replacement.

Rebuild authoritative data from DNSControl and secondary zone transfers without
SQLite backup or restoration. Resolver caches are disposable. Preserve the
ADR-0051 forwarding policy and existing upstream release trains.

## Consequences

Ubuntu and Debian must coexist during rollout. Repository selection and OS
validation must support both without changing existing Ubuntu guests. Recreating
a guest temporarily reduces redundancy; validate each replacement before
continuing. Debian does not change the shared Proxmox kernel or LXC isolation.
Resource reductions require measurements after the full service baseline is
installed. Rollback also recreates guests and regenerates authoritative data.
