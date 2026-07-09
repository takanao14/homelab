# ADR-0017: Renovate automerge go-live adjustments

- **Status:** Accepted
- **Date:** 2026-07-09
- **Related:** amends [ADR-0005](0005-renovate-native-automerge-over-branch-protection.md),
  `renovate.json` (rule rationale lives in its `description` fields). The
  strategy plan (`renovate-automerge-strategy.md` in the private plans repo)
  has been removed now that automerge is live.

## Context

ADR-0005 chose Renovate-native automerge over branch protection. Going live
surfaced refinements that change three of its supporting choices; the core
decision is unchanged.

## Decision

1. **No merge time window.** The planned daytime `automergeSchedule` was
   removed: combined with `minimumReleaseAge`, it added too much latency,
   especially to security updates. Gotcha worth keeping: `schedule` only
   limits when Renovate *creates/updates branches* — it does **not** gate
   merging; that is the separate `automergeSchedule`. This surfaced when the
   first live automerge merged at ~02:00 JST despite a daytime `schedule`.
   Both are now unset.
2. **The guard list applies only where merge == deploy.** ArgoCD auto-syncs
   `k8s/**`, so a merge there is a production deploy and foundation/stateful
   packages stay manual. `ansible/**` and `tf/**` are applied manually
   (`ansible-playbook` / `terragrunt apply`), so a merge is *not* a deploy —
   these paths auto-merge, which is why `openbao` and `seaweedfs` are not on
   the guard list (they live in `ansible/`; their bumps are reviewed at apply
   time). `tf/**` is gated by the `tf-lint` workflow (`terraform fmt -check`
   plus per-module `terraform validate -backend=false` against the pinned
   provider).
3. **Security fast-track.** `vulnerabilityAlerts.minimumReleaseAge: null`
   (security updates skip the 1-day age gate) and
   `osvVulnerabilityAlerts: true`. Coverage is strong for package ecosystems,
   partial for Helm charts and container tags — an accelerator, not a
   guarantee. Foundation/stateful packages keep manual review even for
   security fixes. `minimumReleaseAge` itself was reduced from 3 days to
   1 day.

## Consequences

- A bad app automerge can land at any hour. The cluster is pinned to `main`
  by ArgoCD `automated` + `selfHeal`, so manual `kubectl` fixes are reverted
  within seconds — the only durable rollback is `git revert` of the merge
  commit (or fix-forward). Bounded because only low-blast-radius apps
  auto-merge.
- ADR-0005's alignment risk now covers three workflows: keep the automerge
  `matchFileNames` aligned with the `ansible-lint` / `helm-lint` / `tf-lint`
  trigger paths, or a candidate merges ungated.
