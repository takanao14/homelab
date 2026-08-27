# ADR-0038: Name custom images by role — base, tool, desktop

- **Status:** Accepted
- **Date:** 2026-08-27
- **Related:** [ADR-0037](0037-bake-shared-toolchain-into-server-images.md),
  [ADR-0006](0006-custom-image-pipeline-monorepo-and-seaweedfs-s3.md),
  [`packer/README.md`](../../packer/README.md)

## Context

[ADR-0037](0037-bake-shared-toolchain-into-server-images.md) merged the two
Packer templates and started installing the shared CLI toolchain into every
supported target. That made the `*-custom` images useful out of the box, but it
also removed the option of a genuinely minimal image: every Ubuntu and Rocky
target now carried roughly 3 GB of tooling whether or not the VM wanted it.

Most consumers do not want it. The k0s node VMs (`tf/k8s/*`), `openbao`,
`authentik`, `sssdtest` and `runner1` all pulled `ubuntu-24.04-custom` purely to
get a cloud image with a guest agent and the right timezone.

The names had drifted from the contents as well:

- `*-custom` said "built by us, not upstream" — the distinction that
  `tf/customimage` draws against `tf/cloudimage`. Every image in this pipeline
  is custom, so the suffix identified nothing.
- `*-xrdp` named a service rather than a role. ADR-0037 already noted that the
  suffix had stopped carrying behavioural meaning once the profile moved into
  the var file.

## Decision

**Name every custom image `<distro><version>-<role>`, with three roles that
form a containment ladder, and add the missing minimal role.**

| Role | Contents | `machine_profile` | `install_toolchain` | Disk |
|------|----------|-------------------|---------------------|------|
| `base` | QEMU Guest Agent and the timezone (JST) only | `server` | `false` | 10G |
| `tool` | `base` plus the shared CLI toolchain in `/usr/local` | `server` | `true` | 16G |
| `desktop` | `tool` plus XFCE, XRDP, the Japanese IME and the GUI applications | `desktop` | `true` | 20G |

`base ⊂ tool ⊂ desktop` holds literally: the desktop targets run the same
toolchain the tool targets do, plus the GUI extras.

No new Packer variable is introduced. The three roles are the combinations of
the two switches ADR-0037 already established, and `install_toolchain = false`
is the mechanism `debian13` was already using.

**`base` is not a third machine profile.** It writes `server` to
`/etc/provisioning/machine-profile.local` like any other server image. The
vendored installers validate `TOOL_MACHINE_PROFILE` against `desktop`, `server`
and `auto` and reject anything else, and that file is synced from the dotfiles
repository, so a third value would have to be introduced upstream first. Keeping
`base` a build-time distinction means a later `scripts/provision.sh` run on such
a host installs the server toolchain, which is the desired behaviour anyway.

**`desktop`, not `xrdp`.** XRDP is currently the only GUI access path in this
homelab, so `xrdp` would be the more literal name. It was rejected because it
puts the third suffix on a different axis from the first two — `base` and `tool`
describe how much is baked in, `xrdp` would describe which service is running —
and the containment relationship stops being readable from the names. The cost
is asymmetric: `desktop` is merely less specific and a README line fixes that,
whereas `xrdp` would force another repo-wide rename if a second access method
ever appears.

**Consumers move to the role they actually need**, in the same change:

| Role | Consumers |
|------|-----------|
| `base` | `tf/k8s/{prd,sandbox}` node VMs, `gpuvm`, `authentik`, `openbao`, `runner1`, `sssdtest`, `rocky-server`, `rocky10`, `debian13` |
| `tool` | `toolbox1` |
| `desktop` | `ubuntu-desktop`, `rocky-desktop` |

`base` means the guest agent and the timezone with no repository setup, without
exception. The Rocky `base` targets therefore do not run
`scripts/rocky/epel.sh`; EPEL and CRB remain a `tool`/`desktop` concern. Keeping
the role definition literal was preferred over pre-configuring repositories that
a minimal image has no use for.

## Consequences

- **No VM is recreated by the rename.** `tf/modules/proxmox-vm` carries
  `ignore_changes = [disk[0].file_id]`, so changing an image reference is a
  plan-neutral text change for existing VMs. Only newly created VMs pick up the
  new image. This is what let the rename and the role reassignment land in one
  change instead of two.
- **`tf/customimage` replaces its download resources.** The instance keys
  change, and `checksum` is `ForceNew` in any case, so each node destroys and
  re-downloads. Push the renamed images to the bucket before applying, or the
  download fails at plan time when the sidecar checksum is fetched.
- **The old bucket objects have to be removed by hand.** `push.sh` only
  uploads; `*-custom.img`, `*-xrdp.img` and their `.sha256` sidecars linger in
  the `cloud-images` bucket until deleted with `rclone`.
- **Nine targets instead of six.** The three new ones are `base` images, which
  are the cheapest to build, but `build.sh all` still grows. The build order in
  `ALL_TARGETS` puts the `base` targets first so an interrupted run leaves the
  quickest-to-rebuild targets done.
- **`pve` downloads one more image.** It now needs `ubuntu-24.04-tool` for
  `toolbox1` alongside the base and desktop images. `node1`–`node5` keep four
  images each, all of them `base`.
- **`rocky-9-tool` and `rocky-10-tool` are defined but deployed nowhere.** They
  are buildable and pushable, and `scripts/check-image-refs.sh` requires the
  `push.sh` and `images.hcl` maps to agree, but no `node.hcl` lists them yet.
- **A Rocky `base` host needs CRB enabled before `scripts/provision.sh`.** The
  vendored installer runs `dnf install epel-release` but never enables CRB, and
  EPEL packages — `mosh` among the toolchain baseline — resolve dependencies out
  of it. `packer/README.md` carries the two commands. The durable fix belongs in
  the vendored installer in `takanao14/dotfiles`, not in the image.
- **Server VMs lose their pre-baked toolchain.** This is a partial walk-back of
  ADR-0037's headline consequence for the hosts that moved to `base`: they now
  need a `scripts/provision.sh` run to become usable. That is the intended
  trade — ADR-0037's mistake was applying the toolchain to every server image
  rather than offering the choice.

## Alternatives considered

- **Keep `*-custom` as the name of the new minimal role, and add `*-tool`
  alongside it.** This has the smallest diff by a wide margin: most consumers
  want the minimal image, so their references would not change at all. Rejected
  because the operational impact is identical — the content change flips the
  checksum, which is `ForceNew`, so the download resources are replaced either
  way — while the cost is permanent. `ubuntu-24.04-custom.img` would keep its
  name while silently losing 3 GB of toolchain, leaving no greppable marker that
  the meaning changed, and `custom` would still be naming a different axis from
  `tool` and `desktop`.
- **Make `base` a third `machine_profile` value.** Rejected: the vendored
  installers reject unknown profiles, and the file that validates them is synced
  from an external repository.
- **Split the rename and the role reassignment into two commits.** Rejected once
  `ignore_changes` on `disk[0].file_id` was confirmed — the reassignment carries
  no recreation risk, so the two-step sequencing bought nothing.
