# ADR-0083: Use GNOME Remote Desktop for the current desktop images

- **Status:** Accepted
- **Category:** Provisioning
- **Date:** 2026-10-29
- **Related:** [ADR-0038](0038-image-role-suffixes-base-tool-desktop.md),
  [ADR-0077](0077-select-guest-os-by-role.md),
  [`packer/README.md`](../../packer/README.md)

## Context

ADR-0038 defined the `desktop` role as XFCE, XRDP and the Japanese IME on top
of the toolchain. A Rocky 10 desktop image cannot follow that recipe: EPEL 10
carries no `xrdp`, `xorgxrdp`, Xorg server or Xfce, and RHEL 10 ships GNOME
Remote Desktop as its RDP server instead. Ubuntu 26.04 still packages XRDP, but
GNOME is its default desktop and also ships GNOME Remote Desktop.

## Decision

**The `desktop` images of the current releases, `ubuntu26-desktop` and
`rocky10-desktop`, run the distribution's default GNOME desktop and accept RDP
through GNOME Remote Desktop in system (remote login) mode.** GDM receives the
RDP connection and starts a headless GNOME session for the user who signs in,
so the VM needs no display and serves several users.

- The Japanese IME is the GNOME default IBus: Mozc on Ubuntu, Anthy on Rocky.
- Each VM generates its own RDP TLS key on first boot; the image carries none.
- The system-mode RDP credentials gate GDM before user login. They are a
  secret and are set per host, never baked into the image.
- `ubuntu24-desktop` and `rocky9-desktop` keep XFCE and XRDP until they are
  retired.

## Consequences

- Both current desktop targets share one remote-access script; only the
  package lists differ by distribution.
- A new desktop VM accepts RDP only after its credentials are set; until then
  GDM refuses the connection.
- The GNOME images are larger, so the `desktop` disk grows from 20G to 30G for
  these targets.
- RDP sign-in becomes two-stage: the shared system credentials, then the user's
  own account at the GDM greeter.
- Existing VMs built from the XFCE `ubuntu-26.04-desktop` keep running XFCE;
  `ignore_changes` on `disk[0].file_id` means only new VMs get GNOME.

## Alternatives considered

- **Keep XFCE and XRDP on Ubuntu 26.04 only** — avoids the change there but
  leaves the two current desktop images on different stacks with separate
  scripts, which is what the shared role is meant to prevent.
- **Build XRDP and Xorg for Rocky 10 from source or COPR** — rejected:
  externally built service binaries must be pinned upstream releases, and an
  unmaintained Xorg build is a larger risk than the stock RDP server.
- **GNOME Remote Desktop in per-user mode** — needs an existing local login
  session and per-user keyring setup, which a headless VM does not have.
