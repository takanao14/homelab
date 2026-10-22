# ADR-0049: Manage shared Linux CLI tools with mise

- **Status:** Accepted
- **Amended by:** [ADR-0069](0069-track-dotfiles-vendor-with-renovate.md)
- **Category:** Provisioning
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
- `.chezmoiscripts/run_onchange_after_40_linux_mise.sh.tmpl` installs the pinned
  mise binary and runs `mise install --locked`.
- Cloud Renovate updates versions in `config.toml` and the matching
  `mise.lock` entries.
- The reusable `Refresh mise lockfile` workflow (`mise-lock.yaml`), called by
  pull request validation, regenerates the pinned mise checksums and Linux lock
  entries on a same-repository update branch, verifies that every configured
  tool name remains locked, and commits any difference to that branch.

The homelab repository vendors the config, lockfile, and rendered installer
with `scripts/install/vendor/sync.sh`. `vendor/REVISION` records the exact
dotfiles commit. Packer installs the files as `/etc/mise/config.toml` and
`/etc/mise/mise.lock`, then installs tools under `/usr/local/share/mise`.
Per-user installation uses `~/.config/mise/config.toml` and
`~/.config/mise/mise.lock` instead.

The expected dependency update sequence is:

1. Renovate updates tool versions and lock entries in the dotfiles config.
2. GitHub Actions regenerates and verifies the lockfile on the same pull request.
3. Merge the dotfiles pull request.
4. Homelab Renovate updates the dotfiles commit digest; GitHub Actions syncs
   vendor files on the same pull request (see [ADR-0069](0069-track-dotfiles-vendor-with-renovate.md)).
5. Build at least one representative tool image before merging homelab.

## Rollout status

The initial migration was merged through dotfiles PR 180 and homelab PR 425.
The system lockfile path was corrected through dotfiles PR 183 and homelab PR
427 after the first Packer test showed that mise does not discover
`/etc/mise/config.lock`. The supported system path is `/etc/mise/mise.lock`.

An Ubuntu 24 tool image was built successfully from the correction branch. The
legacy direct-download installer and its rollback documentation were removed
after the soak period, ending duplicate Renovate dependency detection. The
dotfiles scripts were then renumbered into a single execution-order sequence,
which is why the vendored copies carry their chezmoi target names.
Repository checks cover vendored-file drift, shell syntax and lint, Packer
format and validation, and ADR formatting. Renovate's Dependency Dashboard
detects all 43 mise tool declarations.

## Follow-up plan

- Confirm a vendored Renovate mise update installs with `--locked` in a
  representative tool image build.
- Build a representative Rocky Linux tool image. Debian has no tool target (see
  [packer/README.md](../../packer/README.md)), so Debian coverage waits until
  one exists.
- The per-user chezmoi path, including Linux arm64, is outside this ADR's
  acceptance scope; homelab consumes only the vendored copy through Packer on
  Linux amd64.
- Evaluate moving global APT and DNF package installation to Ansible as a
  separate decision; it is not part of the mise migration.
