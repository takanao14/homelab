# ADR-0030: Run Knot Resolver 6 in-house, co-located with dnsdist

- **Status:** Proposed
- **Date:** 2026-07-30
- **Related:** [ADR-0002](0002-dhcp-outside-proxmox-cluster-nodes.md),
  [ADR-0024](0024-shared-proxmox-node-inventory-for-monitoring.md),
  [`ansible/roles/dnsdist`](../../ansible/roles/dnsdist/)

## Context

dnsdist routes internal zones to the pdns secondaries, answers selected RFC 6303
reverse zones locally with NXDOMAIN, and sends everything else to a pool
containing the router, Google Public DNS and Cloudflare DNS. We do not operate
the recursive resolver in that path.

Measurements made while evaluating this decision found:

- Over ten minutes, dist1 sent 109 queries directly to Google and 184 to
  Cloudflare; dist2 sent 37 and 80. Other queries went to the router, whose
  ultimate upstream was not measured.
- Google and Cloudflare returned SERVFAIL for the deliberately bogus
  `dnssec-failed.org`; the router returned NOERROR. Depending on backend
  selection, DNSSEC enforcement is therefore non-deterministic.
- CoreDNS uses `fallthrough in-addr.arpa`, so unanswered pod-address reverse
  lookups reach dnsdist. The current hand-written NXDOMAIN rule prevents those
  queries from leaking beyond the LAN.

Moving recursion in-house avoids disclosing the complete query stream to a
small set of public resolvers, makes DNSSEC enforcement deterministic and puts
RFC 6303 handling in a recursive resolver rather than a load balancer.

The resolver is security-sensitive DNS infrastructure. Selection therefore
cannot be based on the convenience of an OS package alone. Distribution
packages may backport known CVEs while remaining several upstream release
trains behind, omitting correctness fixes, hardening and changed defaults that
DNS operators expect to deploy. The resolver must follow an upstream-supported
stable release train through a signed, automatable package channel.

## Decision drivers

In priority order:

1. A current upstream-supported stable release, not merely a distribution
   security-backport branch.
2. Signed packages for Ubuntu 24.04 with an update path visible to APT and
   Ansible; no locally compiled or manually unpacked binary.
3. DNSSEC validation enforced from the first production query. Bogus data must
   produce SERVFAIL; a global permissive or log-only period is not acceptable.
4. A bounded memory configuration that fits the existing 1 GB dist containers.
5. Native Prometheus metrics and explicit DNSSEC validation-failure logging.
6. A declarative configuration that can be validated before deployment.

## Decision

**Run the current stable Knot Resolver 6 release on dist1 and dist2, bound only
to `127.0.0.1:5353`, as the sole backend in dnsdist's default pool. Install the
`knot-resolver6` package from CZ.NIC's signed upstream repository for Ubuntu
Noble. Keep DNSSEC validation enabled and enforcing.**

As of this decision, Knot Resolver 6.4.1 is the preferred stable upstream
release. The major-specific package name follows compatible 6.x minor and patch
updates without silently crossing to a future major version. A move to Knot
Resolver 7 requires a separate compatibility review.

Use the default 100 MB persistent file-backed cache initially. It is explicitly
sized for personal and small-office deployments and fits the measured memory
headroom. Do not increase it until process RSS, cache use and hit rate show a
reason to do so.

dnsdist remains the client-facing service on port 53 and retains client ACLs,
name-based internal routing, backend health checks and dnstap logging. Knot
Resolver receives only external recursive queries over loopback. Internal
zones remain routed directly to the pdns secondaries; duplicating them as
resolver forwarding policy would create two sources of routing truth.

Enable Knot Resolver's DNSSEC bogus logging and Prometheus metrics. Expose the
metrics listener only to the existing monitoring path; do not expose its
management API generally to the LAN.

## Product evaluation

| Candidate | Upstream package path | Memory and cache | DNSSEC / observability | Result |
|---|---|---|---|---|
| Knot Resolver 6 | CZ.NIC signed Noble repository; `knot-resolver6` follows the stable 6.x train | 100 MB default, shared and persistent | Enforcing by default; bogus logging and native Prometheus | **Selected** |
| PowerDNS Recursor 5.4 | PowerDNS signed `noble-rec-54` repository | Defaults exceed the 1 GB guest when caches fill; explicit caps required | Enforcing modes and native Prometheus | Viable fallback, but needs bespoke memory sizing and repository-channel changes for each release train |
| Unbound current stable | Signed upstream source release, but no NLnet Labs Ubuntu package repository for Unbound | Small default caches | Enforcing validation and failure logs, but no native Prometheus | Rejected: a local source-build and upgrade pipeline would be required |
| Ubuntu Noble Unbound | Ubuntu `main`, maintained 1.19.2 branch | Small default caches | Ubuntu backports CVEs; no native Prometheus | Rejected: not the current upstream-supported release train |

Knot Resolver's lack of a global equivalent to Unbound's
`val-permissive-mode` is intentional here: permissive validation returns bogus
data and conflicts with the requirement to enforce DNSSEC from the first
production query.

PowerDNS Recursor remains the fallback if Knot Resolver fails its canary. It
has a suitable upstream package channel and observability, and its cache limits
can be reduced to fit. Sharing a vendor with dnsdist is useful but not enough
to outweigh Knot Resolver's naturally bounded cache and persistent-cache
behaviour at this query volume.

## Update policy

The upstream repository is part of the security design, not just an
installation convenience.

- Track the latest available `knot-resolver6` version; do not pin a minor or
  patch version indefinitely.
- Extend the version audit to report installed and APT-candidate versions on
  both dist hosts.
- Subscribe to the Knot Resolver release announcement channel and review
  security releases when published.
- Apply resolver upgrades with the existing serial DNS-host package-upgrade
  playbook so only one resolver is restarted at a time.
- Treat an applicable upstream security release as an operational update, not
  as a service redesign. Major-version changes remain manual design decisions.

The CZ.NIC repository is not an Ubuntu `-security` origin, so the existing
unattended-upgrades allowlist does not update it automatically. Do not imply
otherwise. The rollout must either add a safe, staggered automatic policy for
`knot-resolver6` or establish an explicit monitored response target for the
serial upgrade workflow.

The same current-release policy applies to dnsdist in this path. The configured
`dnsdist-20` repository is now a critical-fixes-only train while dnsdist 2.1 is
stable. Upgrade dnsdist to its current stable train before accepting this ADR;
otherwise the frontend would violate the policy used to select the resolver.

## Consequences

- The hand-written `dnsdist_local_reverse_zones` list and its `RCodeAction`
  become redundant. Knot Resolver's built-in special-use and locally-served
  zone policy owns RFC 6303 behaviour.
- `dnsdist_internal_domains` becomes load-bearing. It is the only routing layer
  preventing internal zones from reaching public recursion, so rollout tests
  must cover every configured internal forward and reverse suffix.
- Remove the dnsdist packet cache from the default pool. Knot Resolver owns the
  recursive cache; retaining both makes TTL and failure behaviour harder to
  reason about.
- The dist hosts need outbound UDP and TCP port 53 to root, TLD and
  authoritative servers, rather than only the three current forwarders.
- Cold-cache queries are slower than queries sent to a large public resolver.
  The persistent cache reduces the restart penalty but does not remove
  first-resolution latency.
- Two independent caches reduce aggregate hit rate but preserve host
  independence and match the existing redundant frontend design.
- CZ.NIC becomes an additional package-signing and availability dependency.
  This is accepted because it is the upstream-supported package channel and
  avoids an internal build pipeline.
- The resolver adds a native metrics surface and DNSSEC failure signal instead
  of relying only on journald messages.

## Alternatives rejected

- **Keep public resolvers as fallback backends on each dnsdist host.** dnsdist
  balances rather than treating them as cold standby, so queries would continue
  leaving the LAN and DNSSEC behaviour could diverge by selected backend.
  Availability comes from two complete dnsdist/resolver pairs.
- **Run separate `rec1` and `rec2` containers.** The resolver and dnsdist have
  the same failure domain from a client perspective, and the dist containers
  already co-host dnscollector and vector. Separate guests add addresses,
  Terraform state, inventory and a network hop without useful isolation here.
- **Build current Unbound locally.** This produces an unpackaged binary whose
  version detection, rebuild trigger, dependency patching and provenance become
  our responsibility.
- **Use the Ubuntu Knot Resolver 5 package.** It is a superseded major release
  with Lua configuration and fails the current-upstream criterion.
- **Keep the status quo.** It retains third-party query disclosure,
  non-deterministic DNSSEC enforcement and resolver logic in dnsdist.

## Acceptance prerequisites

Keep this ADR `Proposed` until all of the following are complete:

1. Confirm the CZ.NIC Noble repository signature and verify with
   `apt-cache policy` that `knot-resolver6` resolves to the current stable 6.x
   release.
2. Render and validate the complete YAML configuration with `kresctl validate`.
3. Upgrade dnsdist to its current stable release train and validate its
   configuration before restarting either host.
4. From both dist hosts, verify UDP and TCP reachability to root and non-root
   authoritative servers, including a large DNSSEC response.
5. Canary one dist host at a time. Through dnsdist, test a valid signed domain,
   an unsigned domain, NXDOMAIN, `dnssec-failed.org`, all internal suffixes and
   RFC 6303 reverse space.
6. Confirm that a bogus DNSSEC answer returns SERVFAIL and produces both the
   expected log entry and an observable monitoring signal.
7. Measure total resident memory with the cache active; retain at least the
   operational headroom needed by dnsdist, dnscollector and vector.
8. Verify client failover while either complete dnsdist/resolver pair is
   stopped.
9. Implement and document resolver update detection and the serial security
   update workflow.

## References

- [Knot Resolver installation](https://www.knot-resolver.cz/documentation/latest/gettingstarted-install.html)
- [Knot Resolver releases and compatibility](https://www.knot-resolver.cz/documentation/latest/NEWS.html)
- [CZ.NIC package repository](https://pkg.labs.nic.cz/doc/?project=knot-resolver)
- [Knot Resolver cache sizing](https://www.knot-resolver.cz/documentation/latest/config-cache.html)
- [Knot Resolver DNSSEC configuration](https://www.knot-resolver.cz/documentation/latest/config-dnssec.html)
- [Knot Resolver Prometheus metrics](https://www.knot-resolver.cz/documentation/latest/config-monitoring-stats.html)
- [PowerDNS repository release trains](https://repo.powerdns.com/)
- [PowerDNS Recursor support policy](https://doc.powerdns.com/recursor/appendices/EOL.html)
- [Unbound installation guidance](https://unbound.docs.nlnetlabs.nl/en/latest/getting-started/installation.html)
