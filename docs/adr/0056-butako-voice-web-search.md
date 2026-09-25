# ADR-0056: Internal web search for Butako voice chat

- **Status:** Accepted
- **Date:** 2026-09-25
- **Related:** [ADR-0055](0055-voice-ui-observes-gpu-switch-without-controlling-it.md)

## Context

Butako's local model cannot know current match results or general news.
Search queries can contain personal conversation text if the entire transcript
is forwarded to a public service. Search result text can also contain
instructions that should not control the assistant.

## Decision

The voice API invokes an internal Go MCP service only when the user enables
web search or asks Butako to search. It sends a short generated query rather
than audio, the full ASR transcript, or the model prompt. The MCP service
posts that query to an internal SearXNG instance. SearXNG sends the query to
external search engines. The MCP service may fetch public pages selected by
result ID and restricts redirects, destination addresses, response size, and
time. Search results are untrusted evidence; the app displays sources and
does not claim a date or score it cannot verify.

Search MCP and SearXNG have ClusterIP Services and ingress NetworkPolicies,
with no public route or PVC. SearXNG receives its secret through OpenBao and
External Secrets. Application and proxy access logs must not contain search
queries; the MCP service uses POST for SearXNG. Engine or page hosts can still
observe the query or page request. Container and node logs must be inspected
after rollout because failed engine diagnostics can contain query context.

## Consequences

- Current answers depend on external engine availability and source quality.
- Search failure must leave ordinary local conversation available.
- SearXNG can encounter CAPTCHA and rate limits; unclear or conflicting
  sources must produce an uncertainty response.
- Operators must seed the encrypted key and publish both application images
  before enabling the production chart.
