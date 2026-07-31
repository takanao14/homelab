# ADR-0030: Run Knot Resolver 6 in dedicated resolver containers

- **Status:** Accepted
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

Moving recursion in-house avoids disclosing the complete query stream to a
small set of public resolvers and makes DNSSEC enforcement deterministic. It
also moves RFC 6303 handling into a recursive resolver rather than a load
balancer — but on the measurement above that last point is a tidiness argument,
not a leak being closed. It should not carry weight on its own.

The resolver is security-sensitive DNS infrastructure. Selection therefore
cannot be based on the convenience of an OS package alone. Distribution
packages may backport known CVEs while remaining several upstream release
trains behind, omitting correctness fixes, hardening and changed defaults that
DNS operators expect to deploy. The resolver must follow an upstream-supported
stable release train through a signed, automatable package channel.

The resolver and dnsdist are also separate services with different
responsibilities and lifecycles. dnsdist owns the client-facing address, ACLs,
internal-zone routing, dnstap and backend health; the resolver owns iterative
resolution, DNSSEC validation and a persistent cache. Co-locating them would
put the CZ.NIC package trust, cache memory, filesystem and service failures in
the same LXC as dnsdist's credentials and internal DNS routing. Vector,
journald and node_exporter are fleet infrastructure and may remain co-located,
but application services should otherwise have a single clear purpose.

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

Both dist hosts place resolver1 and resolver2 in the default pool. Use dnsdist's
`firstAvailable` policy with per-frontend ordering: dist1 prefers resolver1 and
uses resolver2 as standby; dist2 prefers resolver2 and uses resolver1 as standby. Health checks
remove an unavailable resolver from selection, so a resolver-only outage fails
over inside dnsdist rather than waiting for a client stub timeout. Keep
`setServFailWhenNoServer` unset. If neither resolver is reachable, dnsdist must
drop the query rather than return SERVFAIL; this still permits a client to try
the other frontend when the failure is an asymmetric network or dnsdist-host
failure.

As of this decision, Knot Resolver 6.4.1 is the preferred stable upstream
release. The major-specific package name follows compatible 6.x minor and patch
updates without silently crossing to a future major version. A move to Knot
Resolver 7 requires a separate compatibility review.

Use the default 100 MB persistent file-backed cache initially. It is explicitly
sized for personal and small-office deployments. Start each resolver LXC with a
1 GB memory limit. Package updates have produced OOM failures in otherwise
comparable 512 MB guests, so 512 MB does not provide sufficient operational
headroom for package management alongside Knot Resolver, Vector and
node_exporter. Do not increase the cache until cgroup memory, cache use and hit
rate show a reason to do so.

dnsdist remains the client-facing service on port 53 and retains client ACLs,
name-based internal routing, backend health checks and dnstap logging. Knot
Resolver receives only external recursive queries from the two dnsdist
frontends. Internal zones remain routed directly to the pdns secondaries;
duplicating them as resolver forwarding policy would create two sources of
routing truth.

Create the guests as independent host-bound Terraform stacks:
`tf/lxc/node2/resolver` for resolver1 and `tf/lxc/node3/resolver` for resolver2.
This follows ADR-0020's host-first tree and keeps resolver lifecycle and plan
output out of the existing stacks that manage dist and authoritative DNS
guests. Provision each guest with only Knot Resolver plus the standard
timezone, node_exporter and LXC logging baseline.

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

This requires adding resolver1 and resolver2 to the `node_exporter_lxc`
inventory group and to the external node_exporter scrape targets. Proxmox OTLP
does not expose in-guest filesystem and memory use, while the resolver's
persistent mmap-backed cache must be measured against the guest cgroup. Note also that
`/metrics/prometheus` returns 404 unless the `prometheus-client` Python package
is installed.

## Product evaluation

| Candidate | Upstream package path | Memory and cache | DNSSEC / observability | Result |
|---|---|---|---|---|
| Knot Resolver 6 | CZ.NIC signed Noble repository; `knot-resolver6` follows the stable 6.x train | 100 MB default, shared and persistent | Enforcing by default; bogus logging and native Prometheus | **Selected** |
| PowerDNS Recursor 5.4 | PowerDNS signed `noble-rec-54` repository | Defaults exceed the intended small resolver guest when caches fill; explicit caps required | Enforcing modes and native Prometheus | Viable fallback, but needs bespoke memory sizing and repository-channel changes for each release train |
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

**Do not add the CZ.NIC repository to the unattended-upgrades allowlist.**
unattended-upgrades runs on each host independently with no coordination
between them, so both resolvers could upgrade and restart inside the same
window — precisely the simultaneous outage that `serial: 1` in
`ops-package_upgrade.yaml` and the dual-resolver design exist to prevent.
Detection is automated instead of application:

- `ops-version_audit.yaml` reports installed and APT-candidate
  `knot-resolver6` versions on resolver1 and resolver2, so a pending upgrade is a
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
`dnsdist_repo_channel: dnsdist-21`
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
  zone policy owns RFC 6303 behaviour. **Remove them only after both dnsdist
  default pools contain only resolver1 and resolver2 and both resolvers have passed
  health and behaviour tests.** Removing them first, while the default pool
  still holds the router and the public resolvers, would reopen the RFC1918
  reverse-lookup path to third parties. On the measurement in Context that path
  currently carries no traffic, so this is precaution rather than an active
  leak — but the ordering costs nothing and the measurement only covers one
  sample window, so keep it.
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
- The resolver containers bootstrap through `dns_external`
  (`192.168.10.1`, `8.8.8.8`) from [`tf/common.hcl`](../../tf/common.hcl), not
  through dnsdist, so package installation and service startup do not depend on
  the resolver itself.
- The resolver hosts need outbound UDP and TCP port 53 to root, TLD and
  authoritative servers, rather than only the three current forwarders.
- The resolver guests need their own Vector configuration containing the Knot
  Resolver and SSH journald units. DNSSEC bogus entries must reach Loki from
  resolver1 and resolver2, not remain only in the local journal.
- Resolver DNS listens on the guest LAN addresses, so it has a larger network
  attack surface than a loopback-only co-located process. Fixed-address binds
  and Knot views restrict service to dist1 and dist2. The management API
  remains unix-socket-only.
- dnsdist-to-resolver traffic is unencrypted Do53 on the trusted LAN. A
  dedicated backend network or DoT would reduce exposure but add interfaces,
  certificates and lifecycle coupling; neither is required by this decision.
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
- resolver1 and resolver2 add two addresses, two Terraform states, inventory
  entries and a network hop. In return, resolver package trust, filesystem,
  cgroup memory, cache and lifecycle no longer share a guest with dnsdist
  credentials and internal routing.
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
- **Co-locate Knot Resolver with dnsdist on dist1 and dist2.** Loopback-only
  exposure and fewer guests are attractive, but this mixes two application
  services with different package sources, state and lifecycles. Resolver cache
  pressure and a CZ.NIC package operation would share the dnsdist cgroup,
  filesystem and credentials; a resolver fault could also disturb internal-zone
  routing that does not otherwise depend on recursion. Cross-cutting logging and
  monitoring agents remain acceptable co-located infrastructure, but the
  resolver gets its own guest.
- **Run resolver1 and resolver2 separately but connect them one-to-one.** This
  provides cgroup and package-trust isolation but does not use the second
  resolver when a single backend fails; external queries would still wait for
  client stub failover. Once the resolver must listen on the LAN anyway,
  allowing both dnsdist addresses and using both healthy backends gives
  materially better availability for little additional configuration.
- **Use a dedicated resolver backend VLAN or DoT between dnsdist and Knot
  Resolver.** Either would reduce trusted-LAN exposure, but a VLAN requires
  additional interfaces and Proxmox network configuration, while DoT introduces
  certificate issuance and renewal into the DNS bootstrap path. Fixed-address
  binds plus a two-address Knot view are sufficient for this trusted homelab
  segment. Revisit if the LAN trust model changes.
- **Proxy `/metrics/prometheus` through the central Caddy on caddy1.** Not
  possible: caddy1 is a separate host and `caddy_upstreams` entries are `IP:port`,
  so it cannot reach a unix socket on a resolver host. Making it reachable means
  binding the management API to a LAN address, and Caddy cannot stop the LAN
  from bypassing it and hitting that unauthenticated API directly. There is no
  host firewall layer in this repository to close that gap.
- **Run a local Caddy on each resolver host restricted to `GET
  /metrics/prometheus`.** This does work — Caddy supports unix-socket upstreams
  and method/path matchers, so the management API could stay on its socket while
  only the metrics path is exposed, and it would give scrape-time freshness that
  the textfile export cannot. Rejected on cost: the caddy role is built for one
  central instance and its Caddyfile template emits whole-host `reverse_proxy`
  lines with no matcher or `respond` support, so it would need generalising. The
  resolver guests need node_exporter for cgroup, cache-filesystem and textfile
  metrics in any case, while a per-host Caddy would be a generalised role serving
  exactly one path. Revisit if the timer interval's staleness ever matters.
- **Build current Unbound locally.** This produces an unpackaged binary whose
  version detection, rebuild trigger, dependency patching and provenance become
  our responsibility.
- **Use the Ubuntu Knot Resolver 5 package.** It is a superseded major release
  with Lua configuration and fails the current-upstream criterion.
- **Keep the status quo.** It retains third-party query disclosure,
  non-deterministic DNSSEC enforcement and resolver logic in dnsdist.

## Acceptance prerequisites

Keep this ADR `Proposed` until all of the following are complete:

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
