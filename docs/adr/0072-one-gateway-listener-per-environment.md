# ADR-0072: One Gateway listener per environment

- **Status:** Accepted
- **Date:** 2026-10-13
- **Related:** [ADR-0010](0010-sandbox-argocd-uses-http-only-gitops-bootstrap.md), [ADR-0011](0011-cilium-gateway-to-envoy-gateway-migration.md), [ADR-0070](0070-no-staging-sandbox-validates-the-cluster-platform.md)

## Context

The shared Gateway rendered an HTTPS and an HTTP listener in prd and only HTTP
in sandbox (ADR-0010). Every prd HTTPRoute pinned `sectionName: https` so it
would not also attach to plain HTTP, and every chart carried a
`gateway.sectionName` value that sandbox overrode to `http`. The prd HTTP
listener had no routes and no redirect; it only returned 404. With sandbox now
limited to the platform set (ADR-0070), the value existed only to work around
that unused listener.

## Decision

Each environment's Gateway has exactly one listener, chosen by
`gateway.listener`: `https` in prd and `http` in sandbox. HTTPRoutes omit
`sectionName` and attach to that listener. prd does not listen on port 80 and
does not redirect HTTP to HTTPS.

## Alternatives considered

- **Keep both listeners and pin `sectionName`.** This is the previous state:
  every chart repeats the listener choice to avoid a listener nothing uses.
  *Rejected.*
- **Keep a prd HTTP listener for an HTTPS redirect.** It needs the HTTP
  listener restricted to `gateway-system` so application routes cannot attach
  to it, plus a redirect route. No client relies on plain HTTP. *Rejected.*

## Consequences

- `http://*.prd.butaco.net` is refused instead of returning 404.
- The chart rejects any `gateway.listener` other than `https` or `http`, so two
  listeners cannot be enabled by accident.
- A future need for plain HTTP or a redirect in prd requires revisiting this
  decision, because routes would then attach to both listeners again.
