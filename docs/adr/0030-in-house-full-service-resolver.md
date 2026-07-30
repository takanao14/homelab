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

Each host's default pool then contains exactly one backend, so dnsdist's
behaviour when that backend is unavailable becomes load-bearing. dnsdist drops
the query when no server in the pool is available; `setServFailWhenNoServer` is
not set today and must stay unset. A dropped query lets the client's stub
resolver time out and retry the other dist host, which is the failover path this
design depends on. Setting it to `true` would return SERVFAIL — a valid answer
that most stub resolvers do not retry elsewhere — turning a single resolver
outage into a hard client failure. Availability still comes from two complete
pairs, but the failover is timeout-driven: while one resolver is down, clients
that reach its host pay the stub timeout on every external query before
retrying.

Enable Knot Resolver's DNSSEC bogus logging and Prometheus metrics. Knot
Resolver 6 serves `/metrics/prometheus` from the management HTTP API, which has
no authentication or authorization of any kind and can reconfigure the resolver
at runtime; there is no separate metrics listener that could be exposed on its
own. Keep the management API on its default unix socket in the resolver rundir
and do not bind it to a LAN address. Bridge the metrics instead: export
`kresctl metrics` periodically into the node_exporter textfile collector
directory (`/var/lib/node_exporter/textfile_collector`), using the same atomic
write-and-rename pattern as the existing rpi_throttled exporter. node_exporter
then carries the resolver metrics over its own scrape, adding no LAN-reachable
listener beyond one that is already a standard part of the fleet.

This requires adding dist1 and dist2 to the `node_exporter_lxc` inventory group;
today that group holds only seaweedfs1, forgejo1 and netbox1, so the dist hosts
have no node_exporter. That addition is worth making on its own terms — the
group exists precisely because Proxmox OTLP does not expose in-guest filesystem
and memory usage for stateful LXC guests, which is exactly what a resolver with
a persistent cache in a 1 GB container needs monitored. Note also that
`/metrics/prometheus` returns 404 unless the `prometheus-client` Python package
is installed.

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

The CZ.NIC repository is not an Ubuntu `-security` origin, and the deployed
unattended-upgrades allowlist covers only the `-security` origins, so
`knot-resolver6` is not updated automatically. Do not imply otherwise.

**Do not add the CZ.NIC repository to the unattended-upgrades allowlist.**
unattended-upgrades runs on each host independently with no coordination
between them, so both resolvers could upgrade and restart inside the same
window — precisely the simultaneous outage that `serial: 1` in
`ops-package_upgrade.yaml` and the two-pair design exist to prevent. Detection
is automated instead of application:

- `ops-version_audit.yaml` reports installed and APT-candidate
  `knot-resolver6` versions on both dist hosts, so a pending upgrade is a
  visible signal rather than something noticed by chance.
- Application stays on the serial workflow: `ops-package_upgrade.yaml` against
  the `dns` group, one host at a time.
- The response target for an applicable upstream security release is one week
  from publication. If that target is routinely missed, the
  current-upstream-release driver is not actually being met and this decision
  should be revisited rather than quietly relaxed.

The same current-release policy applies in principle to dnsdist in this path:
the frontend should not sit on a superseded train while the resolver behind it
is held to a current-upstream requirement. The configured channel is
`dnsdist_repo_channel: dnsdist-20`
([`ansible/roles/dnsdist/defaults/main.yaml`](../../ansible/roles/dnsdist/defaults/main.yaml)).
Whether that train is still current must be checked against PowerDNS'
repository and EOL pages at rollout time rather than asserted here.

That upgrade is **not** a prerequisite of this ADR. A dnsdist major-version move
carries its own configuration-syntax risk and its own rollback story, and
coupling it to the resolver introduction would block one change behind an
unrelated migration. Track it as a separate decision. The only hard requirement
here is that dnsdist's configuration is validated and its installed version
recorded before either host is restarted.

## Consequences

- The hand-written `dnsdist_local_reverse_zones` list and its `RCodeAction`
  become redundant. Knot Resolver's built-in special-use and locally-served
  zone policy owns RFC 6303 behaviour. **Remove them only after both hosts'
  default pools point at Knot Resolver.** Removing them first, while the default
  pool still holds the router and the public resolvers, restores exactly the
  RFC1918 reverse-lookup disclosure the rule was added to stop — the ordering is
  a safety constraint, not a matter of convenience.
- `dnsdist_internal_domains` becomes load-bearing. It is the only routing layer
  preventing internal zones from reaching public recursion, so rollout tests
  must cover every configured internal forward and reverse suffix.
- Remove the dnsdist packet cache from the default pool. Knot Resolver owns the
  recursive cache; retaining both makes TTL and failure behaviour harder to
  reason about.
- The router at `192.168.10.1` leaves the query path along with the public
  resolvers, not just Google and Cloudflare. Kea is configured without DDNS, so
  no LAN hostnames are registered there and none are lost, but any name the
  router answered from its own configuration stops resolving. Confirm during the
  canary that nothing depends on it.
- The dist containers themselves resolve through `dns_external`
  (`192.168.10.1`, `8.8.8.8`) from [`tf/common.hcl`](../../tf/common.hcl), not
  through their own dnsdist, so the resolver has no bootstrap dependency on
  itself and package installation still works while it is down.
- The dist hosts need outbound UDP and TCP port 53 to root, TLD and
  authoritative servers, rather than only the three current forwarders.
- The dist hosts' Vector pipeline ships only the `dnsdist`, `dnscollector` and
  `ssh` journald units. The Knot Resolver units must be added to
  `vector_config.sources.journald.include_units` in
  `ansible/inventories/homelab/group_vars/dnsdist.yaml`, otherwise the DNSSEC
  bogus entries stay on the host and never reach Loki.
- Losing only the resolver, with dnsdist still running, degrades that host to
  timeout-based failover rather than a clean error. It is survivable but slow,
  and it is a distinct failure mode from losing the whole host — both need
  testing.
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
- **Add the peer host's resolver as a second backend in each default pool.**
  This would replace timeout-based failover with immediate in-dnsdist failover,
  but it requires binding Knot Resolver to a LAN address and maintaining an ACL
  on it, giving up the loopback-only exposure. dnsdist's drop-on-no-server
  default already produces working client failover, so the added attack surface
  buys only latency during a single-resolver outage. Reconsider if that outage
  latency proves disruptive in practice.
- **Proxy `/metrics/prometheus` through the central Caddy on caddy1.** Not
  possible: caddy1 is a separate host and `caddy_upstreams` entries are
  `IP:port`, so it cannot reach a unix socket on a dist host. Making it reachable
  means binding the management API to a LAN address, and Caddy cannot stop the
  LAN from bypassing it and hitting that unauthenticated API directly. There is
  no host firewall layer in this repository to close that gap.
- **Run a local Caddy on each dist host restricted to `GET
  /metrics/prometheus`.** This does work — Caddy supports unix-socket upstreams
  and method/path matchers, so the management API could stay on its socket while
  only the metrics path is exposed, and it would give scrape-time freshness that
  the textfile export cannot. Rejected on cost: the caddy role is built for one
  central instance and its Caddyfile template emits whole-host `reverse_proxy`
  lines with no matcher or `respond` support, so it would need generalising. The
  textfile route also adds a resident process to the dist hosts (node_exporter
  is not there today), so the honest difference is not "no new daemon" but which
  daemon: node_exporter is an existing role, deployed unchanged, that brings
  in-guest filesystem and memory metrics these containers currently lack, while
  a per-host Caddy is a generalised role serving exactly one path. Revisit if
  the timer interval's staleness ever matters.
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
3. Validate the dnsdist configuration and record its installed version before
   restarting either host. Upgrading dnsdist is tracked separately and does not
   block this ADR — see *Update policy*.
4. From both dist hosts, verify UDP and TCP reachability to root and non-root
   authoritative servers, including a large DNSSEC response.
5. Canary one dist host at a time. Through dnsdist, test a valid signed domain,
   an unsigned domain, NXDOMAIN, `dnssec-failed.org`, all internal suffixes and
   RFC 6303 reverse space.
6. Confirm that a bogus DNSSEC answer returns SERVFAIL, that the expected log
   entry reaches Loki through the dist hosts' Vector pipeline — not merely
   journald on the host — and that a monitoring signal is observable.
7. Add dist1 and dist2 to the `node_exporter_lxc` inventory group and confirm
   the metrics path end to end: `prometheus-client` installed so
   `/metrics/prometheus` does not 404, the textfile export landing in
   `/var/lib/node_exporter/textfile_collector` and appearing in Prometheus, and
   no LAN-facing bind of the management API on either host.
8. Measure the container's cgroup memory usage (`memory.current`), not process
   RSS alone, with the cache active. The persistent cache is an mmap'd LMDB file
   whose pages are charged to the LXC memory cgroup, so RSS understates the
   footprint against the 1 GB limit. Retain the operational headroom needed by
   dnsdist, dnscollector and vector.
9. Stop `knot-resolver` alone, leaving dnsdist running, and confirm the query is
   dropped rather than answered with SERVFAIL and that clients fail over to the
   other host. This is the failure mode the single-backend pool introduces and
   is not covered by stopping the whole pair.
10. Verify client failover while either complete dnsdist/resolver pair is
    stopped.
11. Implement and document resolver update detection (version audit extension)
    and the serial security update workflow.

## References

- [Knot Resolver installation](https://www.knot-resolver.cz/documentation/latest/gettingstarted-install.html)
- [Knot Resolver releases and compatibility](https://www.knot-resolver.cz/documentation/latest/NEWS.html)
- [CZ.NIC package repository](https://pkg.labs.nic.cz/doc/?project=knot-resolver)
- [Knot Resolver cache sizing](https://www.knot-resolver.cz/documentation/latest/config-cache.html)
- [Knot Resolver DNSSEC configuration](https://www.knot-resolver.cz/documentation/latest/config-dnssec.html)
- [Knot Resolver Prometheus metrics](https://www.knot-resolver.cz/documentation/latest/config-monitoring-stats.html)
- [Knot Resolver management HTTP API](https://www.knot-resolver.cz/documentation/latest/manager-api.html) — unauthenticated; `/metrics/prometheus` needs `prometheus-client`
- [dnsdist downstream servers](https://www.dnsdist.org/guides/downstreams.html) — queries are dropped when no server in the pool is available unless `setServFailWhenNoServer` is set
- [PowerDNS repository release trains](https://repo.powerdns.com/)
- [PowerDNS Recursor support policy](https://doc.powerdns.com/recursor/appendices/EOL.html)
- [Unbound installation guidance](https://unbound.docs.nlnetlabs.nl/en/latest/getting-started/installation.html)
