# ADR-0049: Manage shared Linux CLI tools with mise

- **Status:** Accepted
- **Date:** 2026-09-12
- **Related:** [ADR-0037](0037-bake-shared-toolchain-into-server-images.md),
  [ADR-0038](0038-image-role-suffixes-base-tool-desktop.md)

## Context

The shared Linux CLI toolchain used a large custom downloader. It duplicated
release URL, archive, checksum, version comparison, and installation logic for
each tool. The same desired toolset must support two scopes: a per-user install
from dotfiles on a stock cloud image, and a system baseline baked into a golden
image. Users must also be able to override that baseline with newer local
versions.

macOS already has a suitable package workflow through Homebrew. APT and DNF
packages have different lifecycle and privilege requirements from standalone
CLI releases.

## Decision

Use one mise config and lockfile from the dotfiles repository for standalone
Linux CLI tools. The config pins concrete versions so cloud Renovate can update
them. GitHub Actions regenerates the Linux x64 and arm64 lock entries on the
same update branch. mise itself remains version- and checksum-pinned.
`chezmoi apply` installs mise and its tools per user with `--locked`. Packer
vendors the same installer, config, and lockfile from a pinned dotfiles commit,
then uses mise system mode to install the golden-image baseline under
`/usr/local` with `--locked`.

Shell PATH ordering prefers user mise shims over system mise shims, so a user
can install a different version without changing the image. macOS remains on
Homebrew. APT and DNF packages remain global and keep the existing installer;
moving them to Ansible is a separate change.

## Consequences

- Stock Ubuntu plus dotfiles and a tool/desktop image expose the same declared
  CLI set, with only the installation scope differing.
- Golden images avoid per-user state while retaining local overrides.
- Renovate advances tool versions in reviewable changes; the lock update records
  platform-specific download URLs and checksums.
- Rebuilding the same commit installs the same resolved tool artifacts and does
  not depend on unauthenticated GitHub API availability.
- Tool additions move from custom Bash functions to mise config.
- Packer does not depend on the dotfiles repository during a build because the
  resolved installer, config, and lockfile are committed here.
- Krew remains activated per user; helm-diff uses a shared plugin directory in
  golden images.

## Implementation and update flow

The source of truth is `takanao14/dotfiles`:

- `dot_config/mise/config.toml` contains concrete tool versions.
- `dot_config/mise/mise.lock` contains Linux x64 and arm64 artifact metadata.
- `.chezmoiscripts/run_onchange_after_linux1_mise.sh.tmpl` installs the pinned
  mise binary and runs `mise install --locked`.
- Cloud Renovate updates versions in `config.toml`.
- The `Refresh mise lockfile` GitHub Actions workflow runs on a same-repository
  config update branch, regenerates the lockfile, verifies that no configured
  tool disappeared, and commits the result to that branch.

The homelab repository vendors the config, lockfile, and rendered installer
with `scripts/install/vendor/sync.sh`. `vendor/REVISION` records the exact
dotfiles commit. Packer installs the files as `/etc/mise/config.toml` and
`/etc/mise/mise.lock`, then installs tools under `/usr/local/share/mise`.
Per-user installation uses `~/.config/mise/config.toml` and
`~/.config/mise/mise.lock` instead.

The expected dependency update sequence is:

1. Renovate updates one or more versions in the dotfiles config.
2. GitHub Actions updates and validates the lockfile on the same pull request.
3. Merge the dotfiles pull request.
4. Run the homelab vendor sync against the merged dotfiles commit and open a
   homelab pull request.
5. Build at least one representative tool image before merging homelab.

## Rollout status

The initial migration was merged through dotfiles PR 180 and homelab PR 425.
The system lockfile path was corrected through dotfiles PR 183 and homelab PR
427 after the first Packer test showed that mise does not discover
`/etc/mise/config.lock`. The supported system path is `/etc/mise/mise.lock`.

An Ubuntu 24 tool image was built successfully from the correction branch.
Repository checks cover vendored-file drift, shell syntax and lint, Packer
format and validation, and ADR formatting. Renovate's Dependency Dashboard
detects all 36 mise tool declarations.

## Follow-up plan

- Confirm the first real Renovate mise update pull request receives a matching
  lockfile commit and can install with `--locked`.
- Verify `chezmoi apply` from the current dotfiles `main` on a stock Linux cloud
  image; this validates the per-user path after lockfile adoption.
- Build representative Debian and Rocky Linux tool images. Linux arm64 lock
  entries are generated but have not yet been exercised by Packer.
- Keep the legacy direct-download installer only during the initial soak
  period, then remove it and its rollback documentation to eliminate duplicate
  Renovate dependency detection.
- Evaluate moving global APT and DNF package installation to Ansible as a
  separate decision; it is not part of the mise migration.
