# ADR-0005: Renovate-native automerge instead of branch protection

- **Status:** Accepted
- **Date:** 2026-06-21
- **Related:** [`docs/plans/renovate-automerge-strategy.md`](../plans/renovate-automerge-strategy.md) (rollout, observation mode active), `renovate.json`

## Context

ArgoCD syncs `main` to the clusters pull-based, so **a merge to `main` is an
immediate production deploy**. The only static gate is linting
(`ansible-lint` / `helm-lint`), which checks syntax/schema, not runtime
correctness. Change traffic is asymmetric: the operator pushes directly to `main`
daily (majority); Renovate opens PRs (minority).

Gating the minority (Renovate) with GitHub branch protection would also gate the
majority (manual pushes) — the classic required-check + path-filter deadlock that
would break day-to-day GitOps.

## Decision

Use **Renovate-native automerge** (`automerge: true`, `platformAutomerge: false`,
`ignoreTests: false`): Renovate inspects the PR's checks itself and merges only
when green. **No branch protection**, so direct pushes to `main` stay
unrestricted.

Supporting choices:

- **The lint workflows *are* the gate.** Automerge scope is aligned to the lint
  trigger paths (`matchFileNames: ["k8s/**","ansible/**"]`) so every automerge
  candidate runs a relevant lint; if lint breaks, automerge stops. Job names
  (`ansible-lint`, `helm-lint`) are explicit so the checks are identifiable.
- **Scope = apps automerge, foundation/stateful manual ("Plan C").** Non-major
  app/aux components automerge; cluster-foundation and stateful components
  (cilium, k0s, openebs, cert-manager, argo-cd, openbao, seaweedfs,
  kube-prometheus-stack, loki, external-secrets, plus `custom.rocm`) stay manual
  for minor/major. **All major updates are manual** regardless of package.
  `tf/**` stays manual.
- **`minimumReleaseAge: "1 day"`** filters dead-on-arrival releases while keeping
  homelab app updates moving quickly. This is intentionally shorter than the
  original observation-window plan.
- **Daytime merge window** at go-live (`after 9am and before 6pm every weekday`,
  `Asia/Tokyo`) so unattended deploys land when someone can react.

## Alternatives considered

- **GitHub branch protection / required checks.** *Rejected* — would break
  unrestricted direct pushes to `main`, which are the majority workflow.

## Consequences

- Direct pushes stay frictionless; only Renovate PRs are auto-gated.
- The lint gate is Renovate-internal best-effort, **not** a GitHub-enforced block;
  lint validates syntax/schema only. Mitigated by keeping high-blast-radius
  components on manual review and by `minimumReleaseAge`.
- **Risk to maintain:** if the workflow `paths` and the automerge `matchFileNames`
  ever drift apart, a candidate could merge with no lint — keep them aligned.
- If full enforcement (including manual pushes) is ever needed, migrate to
  "PR-required + always-run lint jobs".
