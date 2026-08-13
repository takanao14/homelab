# ADR-0030: Run Knot Resolver 6 in dedicated resolver containers

- **Status:** Accepted (acceptance prerequisites verified 2026-08-04)
- **Date:** 2026-07-31
- **Related:** [ADR-0001](0001-service-oriented-ansible-playbook-organization.md),
  [ADR-0002](0002-dhcp-outside-proxmox-cluster-nodes.md),
  [ADR-0020](0020-tf-tree-axes-host-vs-cluster.md),
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
  lookups can reach dnsdist. The hand-written NXDOMAIN rule is what would stop
  those queries leaking beyond the LAN.
- **That rule works but is not being exercised.** Measured later on dist1
  (`dnsdist -c -e "showRules()"`): over roughly 4.5 hours and about 22,700
  queries, the RFC 6303 rule matched **zero** times, while the internal-zone
  rules matched 11,420. A manual `dig -x 10.0.0.92` moved the counter from 0 to
  1, so the rule itself is correct — the traffic simply is not arriving. Whether
  CoreDNS' fallthrough actually forwards pod-address reverse lookups here has
  not been confirmed.

In-house recursion reduces third-party query disclosure and makes DNSSEC
enforcement deterministic. Moving RFC 6303 handling out of dnsdist is cleaner,
but measurements show no current leak, so it is not a primary driver.

The security-sensitive resolver must follow an upstream-supported stable train
through a signed, automatable package channel; distribution CVE backports alone
do not provide current correctness and hardening changes.

dnsdist owns client routing, ACLs, dnstap and backend health; the resolver owns
recursion, DNSSEC and cache. Separate LXCs isolate their package trust, state,
memory and failures. Fleet logging and monitoring agents remain co-located.

## Decision drivers

In priority order:

1. A current upstream-supported stable release, not merely a distribution
   security-backport branch.
2. Signed packages for Ubuntu 24.04 with an update path visible to APT and
   Ansible; no locally compiled or manually unpacked binary.
3. DNSSEC validation enforced from the first production query. Bogus data must
   produce SERVFAIL; a global permissive or log-only period is not acceptable.
4. A clear service boundary: resolver package, cache, memory and failures must
   not share the dnsdist LXC.
5. Resolver redundancy below dnsdist, so losing one resolver does not make
   clients wait for stub-resolver failover.
6. A bounded memory configuration in a small dedicated LXC.
7. Native Prometheus metrics and explicit DNSSEC validation-failure logging.
8. A declarative configuration that can be validated before deployment.

## Decision

**Run the current stable Knot Resolver 6 release in two dedicated unprivileged
LXC guests, resolver1 on node2 and resolver2 on node3. Install the `knot-resolver6`
package from CZ.NIC's signed upstream repository for Ubuntu Noble. Keep DNSSEC
validation enabled and enforcing.**

Each resolver binds plain DNS on TCP and UDP port 53 to its fixed guest address,
never to a wildcard. Declarative Knot Resolver views allow queries only from
dist1 and dist2 and refuse all other clients. The management API remains on its
default unix socket and is never bound to a LAN address. Resolver addresses are
backend infrastructure: do not publish them through DHCP or use them directly
as client resolvers.

Both frontends use both resolvers with `firstAvailable`: dist1 prefers resolver1
and dist2 prefers resolver2. Health checks fail over internally. Keep
`setServFailWhenNoServer` unset so an empty pool drops queries, allowing clients
to try the other frontend during asymmetric failures.

As of this decision, Knot Resolver 6.4.1 is the preferred stable upstream
release. The major-specific package name follows compatible 6.x minor and patch
updates without silently crossing to a future major version. A move to Knot
Resolver 7 requires a separate compatibility review.

Start with the 100 MB persistent cache and a 1 GB LXC limit. Comparable 512 MB
guests OOMed during package updates. Increase cache only from cgroup, cache and
hit-rate measurements. The addendum records the later increase to 256 MB.

dnsdist remains client-facing and routes internal zones directly to pdns. Knot
Resolver receives only external recursive queries, avoiding duplicate routing truth.

Create the guests as independent host-bound Terraform stacks:
`tf/lxc/node2/resolver` for resolver1 and `tf/lxc/node3/resolver` for resolver2.
This follows ADR-0020's host-first tree and keeps resolver lifecycle and plan
output out of the existing stacks that manage dist and authoritative DNS
guests. Provision each guest with only Knot Resolver plus the standard
timezone, node_exporter and LXC logging baseline.

Enable DNSSEC bogus logging and Prometheus metrics. Because the unauthenticated
management API can reconfigure the resolver, keep it on its unix socket. Export
metrics atomically to node_exporter's textfile directory instead of opening a
LAN listener.

Add both guests to `node_exporter_lxc` and external scrape targets. Proxmox OTLP
does not expose the needed guest filesystem/cgroup data. Metrics also require
the `prometheus-client` package.

## Product evaluation

| Candidate | Upstream package path | Memory and cache | DNSSEC / observability | Result |
|---|---|---|---|---|
| Knot Resolver 6 | CZ.NIC signed Noble repository; `knot-resolver6` follows the stable 6.x train | 100 MB default, shared and persistent | Enforcing by default; bogus logging and native Prometheus | **Selected** |
| PowerDNS Recursor 5.4 | PowerDNS signed `noble-rec-54` repository | Defaults exceed the intended small resolver guest when caches fill; explicit caps required | Enforcing modes and native Prometheus | Viable fallback, but needs bespoke memory sizing and repository-channel changes for each release train |
| Unbound current stable | Signed upstream source release, but no NLnet Labs Ubuntu package repository for Unbound | Small default caches | Enforcing validation and failure logs, but no native Prometheus | Rejected: a local source-build and upgrade pipeline would be required |
| Ubuntu Noble Unbound | Ubuntu `main`, maintained 1.19.2 branch | Small default caches | Ubuntu backports CVEs; no native Prometheus | Rejected: not the current upstream-supported release train |

Knot Resolver's lack of global permissive validation matches the requirement to
reject bogus data from the first production query.

PowerDNS Recursor remains the canary fallback, but requires more explicit cache
sizing than Knot Resolver.

## Update policy

The upstream repository is part of the security design.

- Track the latest available `knot-resolver6` version; do not pin a minor or
  patch version indefinitely.
- Extend the version audit to report installed and APT-candidate versions on
  resolver1 and resolver2.
- Subscribe to the Knot Resolver release announcement channel and review
  security releases when published.
- Apply resolver upgrades with the existing serial DNS-host package-upgrade
  playbook so only one resolver is restarted at a time.
- Treat an applicable upstream security release as an operational update, not
  as a service redesign. Major-version changes remain manual design decisions.

The CZ.NIC repository is not an Ubuntu `-security` origin, and the deployed
unattended-upgrades allowlist covers only the `-security` origins, so
`knot-resolver6` is not updated automatically. Do not imply otherwise.

**Do not allow unattended CZ.NIC upgrades.** Hosts update independently and
could restart both resolvers together. Automate detection, then apply serially:

- `ops-version_audit.yaml` reports installed and APT-candidate
  `knot-resolver6` versions on resolver1 and resolver2, so a pending upgrade is a
  visible signal rather than something noticed by chance.
- Application stays on the serial workflow: `ops-package_upgrade.yaml` against
  the `dns` group, one host at a time.
- The response target for an applicable upstream security release is one week
  from publication. If that target is routinely missed, the
  current-upstream-release driver is not actually being met and this decision
  should be revisited rather than quietly relaxed.

Apply the same current-train principle to dnsdist, but track major upgrades
separately because their syntax and rollback risks are unrelated. Validate and
record the installed dnsdist version before restart.

## Consequences

- Remove `dnsdist_local_reverse_zones` and `RCodeAction` only after both default
  pools use tested in-house resolvers; earlier removal could expose RFC1918
  reverse queries to remaining public backends.
- `dnsdist_internal_domains` becomes load-bearing. It is the only routing layer
  preventing internal zones from reaching public recursion, so rollout tests
  must cover every configured internal forward and reverse suffix.
- Remove the dnsdist packet cache from the default pool. Knot Resolver owns the
  recursive cache; retaining both makes TTL and failure behaviour harder to
  reason about.
- The router also leaves the query path. Kea has no DDNS, but canary tests must
  confirm no resolver-specific router names are used.
- The resolver containers bootstrap through `dns_external`
  (`192.168.10.1`, `8.8.8.8`) from [`tf/common.hcl`](../../tf/common.hcl), not
  through dnsdist, so package installation and service startup do not depend on
  the resolver itself.
- The resolver hosts need outbound UDP and TCP port 53 to root, TLD and
  authoritative servers, rather than only the three current forwarders.
- The resolver guests need their own Vector configuration containing the Knot
  Resolver and SSH journald units. DNSSEC bogus entries must reach Loki from
  resolver1 and resolver2, not remain only in the local journal.
- Resolver DNS listens on fixed LAN addresses but Knot views allow only dist1
  and dist2; the management API remains unix-socket-only.
- dnsdist-to-resolver traffic is unencrypted Do53 on the trusted LAN; a backend
  network or DoT is deferred.
- Losing one resolver no longer invokes client timeout failover: both dnsdist
  hosts select the healthy peer. Losing both resolvers still leaves internal
  authoritative routing operational on dnsdist, while external queries are
  dropped.
- Cold-cache queries are slower than queries sent to a large public resolver.
  The persistent cache reduces the restart penalty but does not remove
  first-resolution latency.
- Two independent caches reduce aggregate hit rate but preserve resolver
  independence. Local-first ordering keeps normal traffic and cache locality
  aligned with the existing node2/node3 frontend placement.
- Two guests add addresses, Terraform states and a network hop in exchange for
  isolating resolver trust, state, memory and lifecycle from dnsdist.
- The monitoring inventory currently calls dist1 and dist2 `dnsRecursors`.
  Once recursion moves behind them, rename that shared client-facing inventory
  to `dnsFrontends`; resolver1 and resolver2 are backend resolvers and should
  not be published as blackbox client targets.
- CZ.NIC becomes an additional package-signing and availability dependency.
  This is accepted because it is the upstream-supported package channel and
  avoids an internal build pipeline.
- The resolver adds a native metrics surface and DNSSEC failure signal instead
  of relying only on journald messages.

## Alternatives rejected

- **Keep public resolvers as fallback backends on each dnsdist host.** dnsdist
  balances rather than treating them as cold standby, so queries would continue
  leaving the LAN and DNSSEC behaviour could diverge by selected backend.
  Availability comes from two in-house resolvers instead.
- **Co-locate Knot Resolver with dnsdist.** Fewer guests and loopback exposure do
  not justify sharing package trust, state, memory and failure domains. *Rejected.*
- **Connect separate resolvers one-to-one.** A backend failure would still wait
  for client frontend failover instead of using the healthy peer. *Rejected.*
- **Use a backend VLAN or DoT.** Extra network or certificate lifecycle is not
  justified for the trusted segment. Revisit if its trust model changes.
- **Proxy metrics through central Caddy.** It cannot reach remote unix sockets;
  a LAN bind would expose the unauthenticated management API directly. *Rejected.*
- **Run local Caddy metrics proxies.** This preserves the unix socket but requires
  generalizing a central-only role for one path; node_exporter is needed anyway.
  Revisit if textfile staleness matters.
- **Build current Unbound locally.** This produces an unpackaged binary whose
  version detection, rebuild trigger, dependency patching and provenance become
  our responsibility.
- **Use the Ubuntu Knot Resolver 5 package.** It is a superseded major release
  with Lua configuration and fails the current-upstream criterion.
- **Keep the status quo.** It retains third-party query disclosure,
  non-deterministic DNSSEC enforcement and resolver logic in dnsdist.

## Acceptance prerequisites

This ADR was kept `Proposed` until all of the following were complete; all
items below are verified as of 2026-08-04:

1. Add resolver1 under `tf/lxc/node2/resolver` and resolver2 under
   `tf/lxc/node3/resolver`; confirm unique addresses, host placement and clean
   Terragrunt plans before the user applies them.
2. Confirm the CZ.NIC Noble repository signature and verify on both resolvers with
   `apt-cache policy` that `knot-resolver6` resolves to the current stable 6.x
   release.
3. Render and validate the complete YAML configuration with `kresctl validate`.
   Confirm that DNS binds only to each resolver's fixed address, the management
   API remains on its unix socket, dist1 and dist2 are allowed, and an unrelated
   LAN client receives REFUSED.
4. Validate the dnsdist configuration and record its installed version before
   restarting either dnsdist host. Upgrading dnsdist is tracked separately and
   does not block this ADR — see *Update policy*.
5. From resolver1 and resolver2, verify UDP and TCP reachability to root and non-root
   authoritative servers, including a large DNSSEC response.
6. From the dist hosts, canary resolver1 and resolver2 directly before either
   frontend is changed. Then switch dist1's default pool to the two resolvers
   and run the full query matrix before switching dist2: a valid signed domain,
   an unsigned domain, NXDOMAIN, `dnssec-failed.org`, all internal suffixes and
   RFC 6303 reverse space. Repeat through dist2, then confirm with dnsdist
   backend counters that each frontend normally selects its local resolver.
7. Confirm that a bogus DNSSEC answer returns SERVFAIL, that the expected log
   entry reaches Loki through the resolver hosts' Vector pipeline — not merely
   journald on the host — and that a monitoring signal is observable.
8. Add resolver1 and resolver2 to the `node_exporter_lxc` inventory group and
   external scrape targets, and confirm the metrics path end to end:
   `prometheus-client` installed so
   `/metrics/prometheus` does not 404, the textfile export landing in
   `/var/lib/node_exporter/textfile_collector` and appearing in Prometheus, and
   no LAN-facing bind of the management API on either host.
9. Measure each resolver container's cgroup memory usage (`memory.current`), not
   process RSS alone, with the cache active. The persistent cache is an mmap'd
   LMDB file whose pages are charged to the LXC memory cgroup, so RSS
   understates the footprint against the 1 GB limit. Retain sufficient headroom
   for Knot Resolver, Vector, node_exporter and package updates, increasing the
   guest limit before production if necessary.
10. Stop resolver1 alone and, once the health check marks it down, confirm both
    dnsdist frontends use resolver2 without returning SERVFAIL or waiting for
    client timeout; repeat with resolver1 and resolver2 reversed. Internal-zone
    answers must remain available throughout.
11. Stop the node2 pair (dist1 and resolver1) and confirm dist2/resolver2 serves
    internal and external queries; repeat for node3. This tests the physical-host
    failure domains, not only individual services.
12. Make both resolvers unavailable while dnsdist remains running and confirm
    external queries are dropped rather than answered with SERVFAIL, while
    internal zones still resolve through the pdns pool.
13. Rename the monitoring inventory for dist1/dist2 from `dnsRecursors` to
    `dnsFrontends`, and confirm blackbox probes continue to target the
    client-facing dnsdist addresses rather than resolver1/resolver2.
14. Implement and document resolver update detection (version audit extension)
    and the serial security update workflow.

## Addendum (2026-08-10): cache raised to 256 MB on measurement

Measurements now justify increasing the initial 100 MB cache.

Measured over seven days on both resolvers:

| Signal | Normal | During the fault |
|---|---|---|
| Cache hit rate | 90–99% | 36–43% |
| SERVFAIL share of requests | 0% | 47–50% (peak 70%) |
| SERVFAIL seen by clients at dnsdist | 0% | 29–39% |

Both `data.mdb` files reached the limit. `stash failed` logs rose from hundreds
to thousands per day; LMDB resets forced cold recursion, downstream timeouts
peaked near 7/s, and both frontends marked both resolvers down. This repeated
more than fifty times in seven days without alerting.

Both resolvers degrade together despite running on different hypervisors
(resolver1 on node2, resolver2 on node3), because dnsdist splits traffic evenly
and the two caches therefore fill at the same rate.

Measurements ruled out DNSSEC, memory pressure, repeated crashes, authoritative
DNS, disk space and sustained upstream outage.

Raise `knot_resolver_cache_size` to 256 MB. Recheck hit rate and `memory.current`
before further increases because mmap pages count against the 1 GB cgroup.

Alert on the cache hit rate rather than on SERVFAIL alone. The hit rate falls
first and separates cleanly — 90–99% against 36–43% — so it is the leading
indicator; the SERVFAIL share is the outcome, and reaches clients.

## References

- [Knot Resolver installation](https://www.knot-resolver.cz/documentation/latest/gettingstarted-install.html)
- [Knot Resolver releases and compatibility](https://www.knot-resolver.cz/documentation/latest/NEWS.html)
- [CZ.NIC package repository](https://pkg.labs.nic.cz/doc/?project=knot-resolver)
- [Knot Resolver cache sizing](https://www.knot-resolver.cz/documentation/latest/config-cache.html)
- [Knot Resolver DNSSEC configuration](https://www.knot-resolver.cz/documentation/latest/config-dnssec.html)
- [Knot Resolver Prometheus metrics](https://www.knot-resolver.cz/documentation/latest/config-monitoring-stats.html)
- [Knot Resolver management HTTP API](https://www.knot-resolver.cz/documentation/latest/manager-api.html) — unauthenticated; `/metrics/prometheus` needs `prometheus-client`
- [Knot Resolver views and ACLs](https://www.knot-resolver.cz/documentation/latest/config-views.html)
- [dnsdist downstream servers](https://www.dnsdist.org/guides/downstreams.html) — queries are dropped when no server in the pool is available unless `setServFailWhenNoServer` is set
- [dnsdist load-balancing policies](https://www.dnsdist.org/guides/serverselection.html) — `firstAvailable` honours backend order
- [PowerDNS repository release trains](https://repo.powerdns.com/)
- [PowerDNS Recursor support policy](https://doc.powerdns.com/recursor/appendices/EOL.html)
- [Unbound installation guidance](https://unbound.docs.nlnetlabs.nl/en/latest/getting-started/installation.html)
