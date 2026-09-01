# ADR-0037: Bake the shared CLI toolchain into the server images from one Packer template

- **Status:** Accepted
- **Date:** 2026-08-26
- **Related:** [ADR-0001](0001-service-oriented-ansible-playbook-organization.md),
  [`packer/README.md`](../../packer/README.md),
  [`scripts/provision.sh`](../../scripts/provision.sh)

## Context

`packer/` built its images from two near-identical templates. `basic.pkr.hcl`
produced the headless `*-custom` images and installed nothing beyond the guest
agent and the timezone; `xrdp.pkr.hcl` produced the desktop images and installed
the shared CLI toolchain through the `scripts/install` wrappers that
`scripts/provision.sh` also drives.

The two templates differed only in the machine profile they wrote to
`/etc/provisioning/machine-profile.local`, the disk size, and a kitty config
file. Everything else — source block, cloud-init seed, sparsify
post-processor — was duplicated, so any change had to be made twice.

That duplication also meant a VM built from `ubuntu-24.04-custom`,
`rocky-9-custom` or `rocky-10-custom` started with no toolchain at all. Every
such VM needed a full `scripts/provision.sh` run, which downloads roughly 3 GB
of tools per host and needs both internet access and OpenBao credentials.

The vendored installers were already profile-aware: `fonts.sh` and `terminal.sh`
return early on the server profile, and the package installer skips Freelens and
fontconfig there. "The non-GUI subset of the toolchain" therefore needed no new
definition — it is exactly what `TOOL_MACHINE_PROFILE=server` already selects.

## Decision

**Merge both templates into `packer/image.pkr.hcl`, make `machine_profile` a
variable, and run the shared toolchain on every target that supports it.**

- `machine_profile` (`server` by default, `desktop` for the XRDP targets) is
  written to the image and passed to the installers as `TOOL_MACHINE_PROFILE`.
  It is the single switch separating the two image roles.
- [`packer/scripts/common/toolchain.sh`](../../packer/scripts/common/toolchain.sh)
  runs `packages.sh` → `tools.sh` → `terminal.sh` → `fonts.sh` in `global` mode,
  installing into `/usr/local` so the toolchain outlives the build user that
  cleanup deletes. It installs the system-wide kitty defaults only on `desktop`.
- `install_toolchain = false` opts a target out. `debian13` sets it: the
  HashiCorp step resolves an apt suite from `VERSION_CODENAME`, and
  `releases.hashicorp.com` publishes no `trixie` suite.
- `cleanup_script` is now a separate variable on every target. It used to be the
  last entry of `provision_scripts` on the basic targets, which left no place to
  install anything after the distro scripts.
- The three server targets pin `disk_size = "16G"`. That is a ceiling, not a
  preference: `tf/vm/node2/openbao` and `tf/vm/pve/sssdtest` declare 16 GB disks,
  and Proxmox cannot import an image whose virtual size exceeds the target disk.

## Consequences

- **Server VMs come up usable.** The toolchain lands in `/usr/local/bin` with its
  version cache in `/usr/local/share/tool-versions`, so a later
  `scripts/provision.sh` run finds `baseline_satisfies` true and skips the
  reinstall rather than repeating it per host.
- **Rocky images now need EPEL and CRB at build time.** `mosh` is a toolchain
  baseline package and lives in EPEL. That setup moved out of
  `scripts/rocky/xrdp.sh` into `scripts/rocky/epel.sh`, shared by all Rocky
  targets, and handles the dnf4/dnf5 split between Rocky 9 and Rocky 10.
- **Builds got slower and images larger.** Each server image gains roughly 3 GB
  and 10–15 minutes; `build.sh all` grows accordingly, as do the objects in the
  `cloud-images` bucket.
- **Tool versions are now pinned at image build time.** An image that is not
  rebuilt ships whatever `takanao14/dotfiles` pinned when it was built. The
  version cache makes this visible rather than silent: a newer pin reinstalls on
  the next `provision.sh` run.
- **CI validates one template.** `packer-lint.yaml` no longer maps var-file
  names to templates; the `*-xrdp.pkrvars.hcl` naming convention now carries no
  behavioural meaning, since the profile lives inside the var file.
- The HashiCorp installs in `scripts/{ubuntu,rocky}/tools.sh` are now redundant
  with `packages.sh` on the desktop targets. Left in place for this change; they
  are idempotent, and removing them is a separate cleanup.

## Alternatives considered

- **Add the toolchain blocks to `basic.pkr.hcl` and keep two templates** — the
  smaller diff, and it leaves the desktop images untouched. Rejected: it
  duplicates the install chain a second time, in the one place the DRY rule in
  `AGENTS.md` was already being violated.
- **Leave the images bare and rely on `scripts/provision.sh`** — no image bloat,
  and tools are always current. Rejected: it re-downloads the same 3 GB for every
  VM and makes each new VM depend on internet access and OpenBao at first boot.
- **Define a separate "server tool list" for the custom images** — rejected as
  redundant. `TOOL_MACHINE_PROFILE=server` already expresses exactly that list,
  and a second definition would drift from the dotfiles pins.
- **Include `debian13`** — rejected for now; it would make the Debian build fail
  on the HashiCorp apt suite. Revisit when upstream publishes `trixie`.
