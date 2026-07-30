# ADR-0030: Run a full-service resolver in-house (Unbound, co-located with dnsdist)

- **Status:** Proposed
- **Date:** 2026-07-30
- **Related:** [ADR-0002](0002-dhcp-outside-proxmox-cluster-nodes.md),
  [ADR-0024](0024-shared-proxmox-node-inventory-for-monitoring.md),
  [`ansible/roles/dnsdist`](../../ansible/roles/dnsdist/)

## Context

dnsdist routes by name: internal zones to the pdns secondaries, RFC 6303 reverse
space to a local NXDOMAIN rule, everything else to a default pool of
`192.168.10.1` (the router), `8.8.8.8` and `1.1.1.1`. There is no resolver we
operate — recursion is delegated to whichever of those three dnsdist picks.

Three measurements taken while evaluating this reframe the problem.

**Most external queries leave the LAN directly for public resolvers.** Over ten
minutes, dist1 sent 109 queries to Google and 184 to Cloudflare; dist2 sent 37
and 80. The remaining queries went to the router; its ultimate upstream was not
measured. No single directly configured public resolver sees the whole stream
today only because dnsdist splits it between them.

**DNSSEC validation is already happening — but not by us, and not
consistently.** Against Verisign's deliberately-broken `dnssec-failed.org`:

| Resolver | Result | |
|---|---|---|
| dist1 / dist2 (today) | SERVFAIL | bogus rejected |
| `8.8.8.8`, `1.1.1.1` | SERVFAIL | validating |
| `192.168.10.1` (router) | **NOERROR** | **not validating** |

The pool mixes a non-validating backend with two validating ones, so whether a
forged answer is rejected depends on which backend dnsdist happens to choose. By
backend counters, 39% of dist1's external queries and 9% of dist2's go to the
router. The current posture is not "no DNSSEC" — it is *non-deterministic*
DNSSEC, which is harder to reason about than either extreme.

**The reverse-lookup leak reaches inside the clusters.** CoreDNS runs with
`fallthrough in-addr.arpa`, so a pod-address reverse lookup its `kubernetes`
plugin cannot answer is forwarded to this resolver. That is what
`dnsdist_local_reverse_zones` currently handles with a hand-rolled rule — a
recursive-resolver responsibility (RFC 6303) implemented in a load balancer
because there was no resolver to put it in.

## Decision

**Run Unbound on dist1 and dist2, bound to `127.0.0.1:5353`, as the only backend
in dnsdist's default pool. Start DNSSEC validation in permissive mode for a
time-bounded observation period, then move to strict validation.**

The division of labour stays clean: dnsdist keeps the client-facing `:53`, the
ACL, name-based routing, health checking and dnstap; Unbound only ever resolves
what dnsdist hands it over loopback. Internal zones must never reach Unbound —
it would try to recurse them from the root and fail — and dnsdist's existing
`dnsdist_internal_domains` rule is what guarantees that. Internal routing
therefore stays in dnsdist rather than being duplicated as Unbound
`forward-zones`.

Three things decided the implementation, against pdns-recursor and Knot
Resolver:

1. **Memory.** The dist containers are 1 GB LXC guests already running dnsdist,
   dnscollector and vector. Unbound's default caches are `msg-cache-size 4m` plus
   `rrset-cache-size 4m` and it is single-threaded by default — it fits with room
   to spare and needs no tuning. pdns-recursor's own performance guide states it
   uses "a little more than 1GB when the caches are full" under default
   configuration, so it could only be used with explicit cache caps and a
   container bump. Knot Resolver's 100 MB file-backed cache fits, but wants to
   stay resident.
2. **Packaging.** Unbound is in Ubuntu `main`. That removes the apt keyring, pin
   and third-party-repo tasks the dnsdist role needs — less Ansible code, not
   more, despite Unbound being a second vendor's software. Ubuntu 24.04 ships
   `knot-resolver 5.7.2` in `universe`, which is the Lua-configuration
   generation upstream has moved past; using current Knot Resolver 6.x (YAML)
   means adding `deb.knot-resolver.cz` and giving up that advantage.
3. **A time-bounded observation posture is expressible.**
   `val-permissive-mode: yes` performs validation and logs failures, but still
   returns bogus data to the client. This permits measuring breakage before
   enforcement, at the explicit cost of not providing DNSSEC protection during
   that period. Knot Resolver has no equivalent global log-only mode: it
   validates or (against its own documented advice) does not.

### Why validation stays on

Not because a homelab needs DNSSEC in the abstract. Most traffic is HTTPS, where
a forged DNS answer still cannot produce a valid certificate, so the realistic
impact of a spoof is denial or a certificate error rather than silent
interception. APT runs over plain HTTP against `repo.powerdns.com`, but GPG
signatures bound that to a stale-package downgrade. Internal zones are unsigned
and bypass the resolver entirely.

The reason is that moving recursion in-house makes validation *ours*. Today it is
enforced by Google and Cloudflare when dnsdist selects either of them, but not
when it selects the router. A local resolver with validation disabled would make
acceptance of bogus data deterministic and would therefore be a regression from
the current mixed posture. Recursion would also newly traverse the ISP link
directly to many authoritative servers rather than to a small set of anycast
resolvers.

Permissive mode has the same client-visible failure semantics as non-enforcing
validation for bogus data: the client receives it. It is accepted only as a
temporary rollout instrument that produces evidence before strict enforcement,
not as preservation of the current protection and not as the desired steady
state.

## Alternatives considered

- **pdns-recursor** — one vendor across auth, dnsdist and recursor, one apt
  repository, and `serve-rfc1918` on by default. Rejected on memory: its default
  cache sizing alone exceeds the container it would run in, and capping caches to
  fit trades away the "matches upstream defaults" benefit that motivated it.
  *Reconsider if the dist containers are ever resized for another reason.*
- **Knot Resolver** — the best-designed of the three: cache shared across workers
  and persisted to disk, so a restart or upgrade does not start cold, and DNSSEC
  validating by default since 4.0. Rejected on the Ubuntu 5.7.2 / upstream 6.x
  split (templating a Lua config from Ansible for a superseded generation, or a
  third-party repo for the current one) and on having no permissive validation
  mode. *The persistent cache is the one axis where the chosen option is worse.*
- **A separate LXC pair (`rec1`/`rec2`)** — matches the one-service-per-container
  pattern used for caddy, log1, forgejo1, netbox1 and seaweedfs1. Rejected
  because the resolver's lifecycle is inseparable from dnsdist's (neither is
  useful without the other), and because the dist containers already co-host
  dnsdist, dnscollector and vector. It would add two IPs, a Terraform stack, an
  inventory group and a network hop for no isolation that matters here.
- **Keeping the public resolvers in the default pool as fallback** — rejected:
  dnsdist balances rather than prioritises, so queries would keep leaving the LAN
  and the validation inconsistency would persist. Availability comes from having
  two dist hosts, each with its own resolver.
- **Keeping the router as a fallback** — rejected for the same reason, and it is
  the measured non-validating backend.
- **Status quo** — rejected: it is the thing the measurements above describe.

## Consequences

- **The hand-rolled RFC 6303 rule becomes redundant and should be deleted.**
  `dnsdist_local_reverse_zones` and the `RCodeAction` block that consumes it are
  replaced by Unbound's default handling of locally-served zones. Keep the
  behaviour in one place, not both.
- **`dnsdist_internal_domains` becomes load-bearing for correctness, not just
  routing.** It is the only thing preventing internal names from being sent to a
  resolver that would try to recurse them from the root.
- **The dnsdist packet cache on the default pool should be reviewed.** In front
  of a loopback resolver with its own cache it adds little, and
  `dnsdist_packet_cache_max_ttl: 86400` can serve data staler than Unbound
  would. Removing it also removes a layer from what becomes a three-tier cache
  (CoreDNS `cache 30` → dnsdist → Unbound).
- **The dist containers need outbound UDP and TCP port 53 to the internet.**
  Today they only ever talk to `192.168.10.1`, `8.8.8.8` and `1.1.1.1`;
  recursion reaches root and TLD servers directly.
- **Cold-cache lookups get slower.** A first query for a domain performs full
  recursion instead of hitting a warm public resolver. Two independent caches
  (one per dist host) rather than one also lowers the aggregate hit rate — an
  acceptable trade at this query volume, and it keeps the hosts independent.
- **Root trust anchor maintenance is inherited.** Ubuntu's `unbound` package
  installs `root-auto-trust-anchor-file.conf` and uses the distribution-provided
  root trust anchor with Unbound's RFC 5011 tracking. `unbound-anchor` is a
  separate package in Ubuntu 24.04 `universe`, not part of the `unbound` package;
  do not claim or depend on it unless the role installs it explicitly. During
  rollout, inspect the effective configuration with `unbound-checkconf` and
  verify the trust-anchor file's ownership and writeability on each host.
- **Unbound has no Prometheus endpoint, and Ubuntu packages no exporter for it.**
  pdns-recursor exposes metrics natively, so this is a real cost of the choice
  made here — one not weighed when the comparison was made on memory and
  packaging. Observation goes through journald instead: the dist hosts already
  ship it to Loki with vector, so adding `unbound` to `include_units` alongside
  `val-log-level: 1` surfaces validation failures. Under permissive mode those
  failures never appear as SERVFAIL, which makes the log the only signal. The
  rollout is not complete until a saved Loki query counts those failures by host
  and queried name. An `unbound_exporter` binary is deliberately not used:
  unpackaged, invisible to Renovate, and ADR-0028 already recorded what that
  costs.

### Deferred: strict validation

`val-permissive-mode: yes` is a rollout state, not the end state. The observation
period starts only after the Loki query above is saved and verified with a known
failure such as `dnssec-failed.org`. After 14 consecutive days with no
unexplained validation failures, change to strict validation
(`val-permissive-mode: no`). If failures occur, investigate the affected domains;
fix the cause or record a narrowly scoped exception rather than extending
permissive mode without a new decision. Verify strict mode against both a valid
signed domain and `dnssec-failed.org`.

### Prerequisites

Memory and partial network reachability were measured on the hosts while
evaluating the proposal:

- **Memory headroom is ample.** `free -m` reports 760 MB available on dist1 and
  791 MB on dist2 (of 1024 MB total, ~250 MB in use). Unbound's default caches
  are 8 MB against that, so no container resize is required — and there is room
  to raise the cache well beyond the default if the hit rate warrants it.
- **UDP reachability to one root server works.** Both hosts reach `198.41.0.4`
  (a.root-servers.net) directly, receiving NOERROR with the full root NS set in
  ~290 ms. The response was 811 bytes with EDNS bufsize 4096 advertised. This
  proves UDP reachability only to that address; it does not establish TCP/53,
  reachability to arbitrary authoritative servers, large DNSSEC responses or
  successful full recursion.

Before accepting this ADR, verify from both dist hosts:

1. UDP and TCP queries to root and non-root authoritative servers.
2. `dig +dnssec DNSKEY . @198.41.0.4` with no truncation or path failure.
3. Cold-cache recursive lookups through the local Unbound instance for signed,
   unsigned, nonexistent and deliberately bogus domains.
4. The same lookups through dnsdist, confirming that internal domains still use
   the pdns pool and external domains use only the loopback Unbound backend.
