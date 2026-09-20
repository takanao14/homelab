# ADR-0051: Forward recursive cache misses over authenticated DNS-over-TLS

- **Status:** Accepted
- **Date:** 2026-09-19
- **Accepted:** 2026-09-20
- **Related:** [ADR-0030](0030-in-house-full-service-resolver.md)

## Context

ADR-0030 made resolver1 and resolver2 full-service recursive resolvers to keep
query data local and make DNSSEC enforcement deterministic. On 2026-09-19,
outbound DNS traffic to authoritative servers began suffering packet loss.
Both resolvers returned SERVFAIL for roughly 30% of queries while dnsdist
recorded sustained downstream timeouts. Direct tests from resolver1, resolver2,
and dist1 reproduced UDP port 53 timeouts to multiple unrelated external DNS
servers. CPU, cache capacity, service restarts, and local firewall rules did not
explain the failure.

Authenticated DNS-over-TLS tests from both resolvers to Google Public DNS
succeeded on every connection. The tests covered normal answers, signed data,
DNSSEC-bogus data, and NXDOMAIN. Knot Resolver forwarding retains its local
persistent cache.

Forwarding to the bgw1 router is not suitable for this mitigation. Its UDP DNS
service timed out during the incident, and its previous treatment of
DNSSEC-bogus answers did not meet the deterministic validation requirement.

## Decision

Add optional forwarding to the Knot Resolver role. An empty forwarder list
preserves full-service recursion. Configure resolver1 as a canary to forward the
root subtree to Google Public DNS over authenticated TLS using both `8.8.8.8`
and `8.8.4.4` with the certificate name `dns.google`.

Keep the existing 256 MB persistent cache, client views, metrics, and dnsdist
routing unchanged. Google performs DNSSEC validation and its answers are trusted
over authenticated TLS. Local revalidation is disabled because it produced
`NSEC Missing` for valid CNAME chains such as `ios.chat.openai.com`. Do not clear
the cache when switching modes. Validate cache misses explicitly during rollout
so cached answers do not conceal an upstream failure.

After the canary meets the acceptance criteria, move the forwarder definition
from resolver1 host variables to the shared resolver group variables and apply
the change to resolver2 serially.

## Consequences

- Cache hits continue to be answered locally. Only cache misses are disclosed
  to the configured public resolver.
- resolver1 and resolver2 retain independent persistent caches and failure
  domains, but share Google Public DNS as an upstream dependency after rollout.
- DoT avoids the failing UDP path and authenticates the upstream, but does not
  repair or identify the underlying network fault.
- DNSSEC enforcement depends on Google Public DNS while forwarding is enabled.
- Google can observe cache-miss query names. This reverses ADR-0030's preference
  to avoid third-party query disclosure and is accepted only after the canary
  is reviewed.
- Removing the forwarder definition and reapplying the role restores
  full-service recursion without clearing cached data.

## Acceptance criteria

1. Validate the rendered configuration with `kresctl validate` and run the
   resolver playbook in check mode for resolver1.
2. Apply only to resolver1 and confirm the service reloads without a restart or
   loss of metrics freshness.
3. From both dnsdist hosts, verify signed, unsigned, NXDOMAIN, DNSSEC-bogus,
   internal-zone, and RFC 6303 reverse queries.
4. Confirm new cache-miss traffic uses authenticated TCP port 853 and does not
   contact authoritative servers directly.
5. Observe latency, downstream timeouts, SERVFAIL share, and cache hit rate for
   at least 30 minutes before enabling forwarding on resolver2.
6. Remove the resolver1 host override and restore full-service recursion if the
   canary increases failures or does not materially improve the incident.
