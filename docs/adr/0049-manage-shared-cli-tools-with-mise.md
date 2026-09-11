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

Use one version-pinned mise config from the dotfiles repository for standalone
Linux CLI tools. `chezmoi apply` installs mise and its tools per user. Packer
vendors the same installer and config from a pinned dotfiles commit, then uses
mise system mode to install the golden-image baseline under `/usr/local`.

Shell PATH ordering prefers user mise shims over system mise shims, so a user
can install a different version without changing the image. macOS remains on
Homebrew. APT and DNF packages remain global and keep the existing installer;
moving them to Ansible is a separate change.

## Consequences

- Stock Ubuntu plus dotfiles and a tool/desktop image expose the same pinned CLI
  set, with only the installation scope differing.
- Golden images avoid per-user state while retaining local overrides.
- Tool additions and upgrades move from custom Bash functions to mise config.
- Packer does not depend on the dotfiles repository during a build because the
  resolved installer and config are committed here.
- Krew remains activated per user; helm-diff uses a shared plugin directory in
  golden images.
