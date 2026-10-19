# ADR-0076: One Terragrunt stack per DNS role on each host

- **Status:** Accepted
- **Category:** Network & DNS
- **Date:** 2026-10-19
- **Related:** [ADR-0020](0020-tf-tree-axes-host-vs-cluster.md),
  [ADR-0030](0030-in-house-full-service-resolver.md),
  [ADR-0075](0075-debian-for-dns-lxc-guests.md)

## Context

ADR-0030 placed the resolvers in their own host-bound stacks to keep their
lifecycle and plan output separate. The older `dnsserver` stacks still combined
dnsdist frontends with PowerDNS authoritative guests, so changing one role
showed the other in plans, and every rebuild required `-target` to avoid
replacing a peer.

## Decision

Each host has one Terragrunt stack per DNS role: `dnsdist`, `pdns` and
`resolver` under `tf/lxc/<host>/`. A stack holds only guests of that role; ns1
and ns3 share `node2/pdns`. Containers keep their addresses, resources and
Terraform resource keys. Move existing state between stacks with
`state mv`, not `import`, so provider-computed attributes are preserved and no
container is replaced.

## Consequences

Role changes plan only that role's guests. `-target` remains necessary only in
`node2/pdns`. Migration temporarily requires the old `dnsserver` state objects
until both new stacks plan with no changes.
