# ADR-0051: Forward recursive cache misses over authenticated DNS-over-TLS

- **Status:** Accepted
- **Category:** Network & DNS
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

Add explicit `recursive` and `emergency_forward` modes to the Knot Resolver
role. `recursive` remains the role default. During recurring external DNS
reachability incidents, `emergency_forward` forwards the root subtree to Google
Public DNS over authenticated TLS using both `8.8.8.8` and `8.8.4.4` with the
certificate name `dns.google`.

Keep the existing 256 MB persistent cache, client views, metrics, and dnsdist
routing unchanged. Google performs DNSSEC validation and its answers are trusted
over authenticated TLS. Local revalidation is disabled because it produced
`NSEC Missing` for valid CNAME chains such as `ios.chat.openai.com`. Keep the
cache when entering `emergency_forward`, and validate cache misses explicitly so
cached answers do not conceal an upstream failure.

Return production to `recursive` only after 14 consecutive days in which the
UDP and TCP root-server probes from both resolvers succeed at least 90% of the
time in every 5-minute window. The `AuthoritativeDns*` alerts are not a
sufficient signal; short path losses stay below their `for` durations. Mode
changes are made in shared resolver group variables. The playbook's `serial: 1`
only orders hosts, so apply to resolver1 with `--limit` and observe it for 24
hours before running it for resolver2. Restore `emergency_forward` on a host
whose dnsdist client SERVFAIL ratio exceeds 2% in any 5-minute window.

## Consequences

- Cache hits continue to be answered locally. Only cache misses are disclosed
  to the configured public resolver.
- resolver1 and resolver2 retain independent persistent caches and failure
  domains, but share Google Public DNS as an upstream dependency after rollout.
- DoT avoids the failing UDP path and authenticates the upstream, but does not
  repair or identify the underlying network fault.
- DNSSEC enforcement depends on Google Public DNS while forwarding is enabled.
- Google can observe cache-miss query names. This reverses ADR-0030's preference
  to avoid third-party query disclosure and is accepted only while
  `emergency_forward` is active.
- Setting the mode to `recursive` restores full-service recursion. Because the
  emergency mode trusts upstream DNSSEC validation, the serial resolver
  playbook clears each cache automatically after loading the recursive
  configuration and before continuing to the peer.

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
6. Restore `recursive` mode if the canary increases failures or does not
   materially improve the incident. Confirm that the role cleared cache data
   accepted while local validation was disabled.

## Addendum (2026-10-22): rollout retrospective

resolver1 reloaded into forwarding at 2026-09-19 14:57 UTC and resolver2 at
15:58 UTC. Both reloads replaced the kresd workers without restarting the unit;
textfile metrics stayed under 46 seconds old, and per-worker counters reset
once. The direct UDP failure had subsided by 14:10 UTC, so the canary could not
show an improvement, yet resolver2 was enabled although criterion 6's rollback
condition was met. From 16:00 UTC, local revalidation failed Cloudflare CNAME
chains for OpenAI names and raised hourly client SERVFAIL to 18-57% until it
was disabled at 23:02 UTC.

## Addendum (2026-10-22): failed return to recursive

The original exit condition (no firing probe alert for 14 days) was met, and
both resolvers returned to `recursive` at 11:02 and 11:42 UTC. In the preceding
14 days the probes averaged 99.76% (UDP) and 99.86% (TCP), but their worst
5-minute windows fell to 13% and 30%. After the change the same path losses
reached clients three times: 12:00-13:30, around 15:30, and 20:55-21:50 UTC.
The last episode raised resolver SERVFAIL to 48-50% and client SERVFAIL to 7%
in 5-minute windows, with 2,100-2,600 downstream timeouts per dnsdist host in
30 minutes. No probe or dnsdist alert fired. Both resolvers returned to
`emergency_forward` at 22:19 UTC, and the exit condition above replaced the
alert-based one.
