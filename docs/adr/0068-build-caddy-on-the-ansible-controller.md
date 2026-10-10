# ADR-0068: Build Caddy on the Ansible controller

- **Status:** Accepted
- **Date:** 2026-10-10

## Context

Caddy needs the `caddy-dns/cloudflare` plugin, which ships no release binary.
The caddyserver.com download API builds on demand: the plugin version cannot
be pinned, the output is stripped after the build and cannot be reproduced from
source, and a server-side rebuild changes the bytes for the same Caddy version.
Unpinned downloads let every playbook run replace and restart Caddy.

## Decision

Build Caddy with xcaddy on the Ansible controller, pinning the Caddy, plugin,
xcaddy, and Go toolchain versions, and require the result to match a pinned
`caddy_sha256`. A fixed set of pins produces a byte-identical binary, so the
hash identifies the reviewed build. Hosts are updated only when their binary
hash differs.

Publishing a CI-built artifact was rejected: it adds a release pipeline and a
second artifact store for a single host without improving the guarantee.

## Consequences

The controller needs `go` and network access to the Go module proxy when a pin
changes. Renovate tracks the four pins but cannot compute the hash, so Caddy
updates are manual-review PRs that update `caddy_sha256`.
