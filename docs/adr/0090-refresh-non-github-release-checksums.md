# ADR-0090: Refresh non-GitHub release checksums on Renovate PRs

- **Status:** Accepted
- **Category:** Repository & delivery
- **Date:** 2026-11-22
- **Related:** [ADR-0069](0069-track-dotfiles-vendor-with-renovate.md)

## Context

Renovate updates the version and SHA-256 together for GitHub-hosted release
binaries through `github-release-attachments`. Forgejo and the Forgejo runner are
published on Codeberg and code.forgejo.org, which that datasource cannot read, so
Renovate bumped only their versions and left stale checksums on PR branches and
on `main`.

## Decision

Keep pin verification and refresh in `scripts/release-checksums.py`. A
same-repository Renovate PR changing only those role defaults triggers a
`pull_request_target` workflow that runs the base revision's script. It fetches
the upstream `.sha256` file, requires the downloaded binary to match it, and
commits the new pin to the PR branch with `GITHUB_TOKEN`. It then dispatches
Ansible Lint against the resulting commit, as ADR-0069 does for vendor checks.
The PR-triggered checksum check defers to this refresh for such PRs. Manual
review of these binary updates remains.

## Consequences

The checksum is only as trustworthy as the upstream release host, which already
serves the binary. A Renovate PR that also changes other paths is rejected and
needs a manual checksum update. The refresh and dispatched checks require the
workflows to be present on the default branch.
