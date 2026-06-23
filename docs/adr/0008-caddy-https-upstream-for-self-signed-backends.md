# ADR-0008: Caddy re-encrypts to HTTPS upstreams that enforce TLS (TrueNAS)

- **Status:** Accepted
- **Date:** 2026-06-24
- **Related:** [`ansible/roles/caddy/README.md`](../../ansible/roles/caddy/README.md), [`k8s/homepage/README.md`](../../k8s/homepage/README.md)

## Context

Caddy terminates the external Let's Encrypt TLS for internal services and reverse
proxies to their backends. Every upstream was wired as plain HTTP
(`reverse_proxy <host>:<port>`), which is fine for backends that only speak HTTP.

TrueNAS SCALE is different: it serves its own UI/API over HTTPS with a **local
self-signed certificate**, and (25.04+) **automatically revokes any API key that
authenticates over a non-TLS connection**. Because TLS is terminated at Caddy and
two independent connections exist —

```
client --HTTPS(Let's Encrypt)--> Caddy --HTTP(:80)--> TrueNAS
```

— the Caddy→TrueNAS leg was plaintext. From TrueNAS's view the homepage widget's
API key arrived over HTTP, so TrueNAS revoked it (`Attempt to use over an insecure
transport`). The client-facing HTTPS being valid is irrelevant; the two legs are
unrelated.

## Decision

Make the reverse-proxy scheme **per-upstream** instead of hard-coded HTTP. The
`caddy_upstreams` entries gained two optional fields, `scheme` (default `http`,
preserving every existing backend) and `tls_insecure` (default `false`). TrueNAS
is configured as an HTTPS upstream with verification skipped:

```yaml
- hostname: truenas-ui.home.butaco.net
  backend: 192.168.20.10:443
  scheme: https
  tls_insecure: true
```

`tls_insecure_skip_verify` applies **only** to the internal Caddy→backend leg; the
client still validates Caddy's public Let's Encrypt certificate. This keeps the
external trust chain intact while satisfying the backend's TLS-only requirement.

## Alternatives considered

- **Issue a real internal-CA cert for TrueNAS and verify it in Caddy** — removes
  the skip-verify, but adds an internal CA / cert-distribution lifecycle for a
  single LAN-internal hop. *Rejected* as disproportionate; remains the upgrade
  path if backend identity verification becomes a requirement.
- **Disable TrueNAS's HTTP→HTTPS enforcement / allow HTTP API keys** — would let
  the plaintext leg keep working, but defeats the backend's security control and
  is not configurable on modern SCALE anyway. *Rejected.*
- **Bypass Caddy and point homepage straight at TrueNAS HTTPS** — avoids the
  re-encrypt, but loses the single Let's Encrypt ingress and uniform hostname
  scheme, and pushes self-signed-cert handling into every client. *Rejected.*

## Consequences

- Caddy can now front any TLS-only backend; the default-`http` keeps all other
  upstreams unchanged (DRY, backward compatible).
- The Caddy→backend leg is encrypted but **unauthenticated** for `tls_insecure`
  upstreams — acceptable on the trusted LAN, but a documented trade-off.
- General rule for this repo: **any backend that enforces TLS (especially one that
  revokes credentials used over HTTP) must be added as an `https` upstream**, not
  plain HTTP. Watch for this when onboarding future self-signed services.
