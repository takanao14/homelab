# Homelab repository instructions

## Working agreements

- Communicate in Japanese. Use English in code, comments, and documentation;
  use ASCII filenames.
- Prioritize maintainability and DRY infrastructure code. Consolidate shared
  Terraform logic in modules and inject environment differences through
  `terragrunt.hcl`.
- The active environments are `prd` and `sandbox`. The `dev` environment was
  retired by ADR-0019; do not reintroduce it without a new architecture decision.

## Writing discipline

- Keep comments and documentation proportional to the code change. Make the
  smallest documentation update that leaves the repository accurate.
- Add a comment only when it explains a non-obvious reason, invariant, safety
  constraint, compatibility requirement, or operational hazard. Do not narrate
  what the code or configuration already states.
- Do not add docstrings, examples, background sections, or step-by-step
  explanations to self-explanatory internal code.
- Prefer replacing outdated text over appending historical commentary. READMEs
  describe the current state, ADRs preserve decision rationale, and private
  plans contain rollout steps. Do not duplicate the same explanation across
  them.
- Update an existing paragraph or list before creating a new section or file.
- Preserve necessary detail for secrets, destructive operations, recovery,
  infrastructure invariants, and externally consumed interfaces.
- Before finishing, review the prose diff and remove every sentence that does
  not add distinct operational or design information.

## Repository boundaries

- Check `docs/adr/` before proposing structural changes and add an ADR for a
  significant design decision.
- `docs/plans` and `docs/md` are gitignored symlinks to separate private
  repositories and may be absent. Edit them in place when needed, but commit
  changes in their own repositories, never this one.
- SOPS+AGE-encrypted `*.sops.env` and `*.sops.yaml` files in this repository are
  the source of truth. OpenBao holds only kubeconfigs, `.env` files, and the AGE
  key managed through `scripts/secrets/admin`; it does not mirror the encrypted
  files. Never hardcode, print, or commit decrypted secret values.

## Infrastructure rules

- After changing infrastructure code, run the relevant non-destructive
  validation and summarize its impact. This includes `terragrunt plan` for
  affected Terraform environments, `ansible-playbook --check`, and
  `helmfile template` as applicable.
- Run `terraform fmt` and `terragrunt hclfmt` after HCL changes. Lint and render
  Helmfile and Helm values changes before handoff.
- Before rendering a wrapper chart, run `helm dependency update`, never
  `helm dependency build`. `Chart.lock` and `charts/*.tgz` are ignored local
  artifacts and do not represent how Argo CD resolves `Chart.yaml`.
- Pin both the release version and SHA-256 for externally built service
  binaries. Do not use `latest`, mutable branch archives, or target-side source
  builds.
- Before editing a values file under `k8s/`, confirm that Argo CD actually reads
  it with:

  ```sh
  kubectl -n argocd get application <name> \
    -o jsonpath='{.spec.source.helm.valueFiles}'
  ```

  For multi-source applications, inspect `.spec.sources[].helm.valueFiles`.
  Render with exactly those files; do not validate an extra `-f` that Argo CD
  does not use.
- Update `README.md` or `AGENTS.md` when the repository structure changes.
