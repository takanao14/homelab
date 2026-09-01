# ADR-0031: Run a browser editor on toolbox1, relocated to pve

- **Status:** Accepted
- **Validated:** 2026-09-01; the Terraform stack, Ansible role, routing records,
  and authenticated code-server endpoint are present.
- **Date:** 2026-07-31
- **Related:** [ADR-0020](0020-tf-tree-axes-host-vs-cluster.md),
  [ADR-0021](0021-relocate-prd-control-plane-to-node4.md),
  [ADR-0025](0025-run-meshcentral-outside-managed-cluster.md),
  [`ansible/roles/code_server`](../../ansible/roles/code_server/README.md),
  [`tf/vm/pve/toolbox1`](../../tf/vm/pve/toolbox1/terragrunt.hcl)

## Context

The `claude` and `codex` CLIs run on `toolbox1`. Reaching that work from an
iPad needs a browser-based editor: iPadOS has no VS Code application, so every
option is "VS Code rendered in a browser". The requirement is to read and edit
the agents' working tree and to drive the agents from an integrated terminal.

`toolbox1` sat on node3 with 8 GiB. Measured at the time of this decision,
node3 has 15.4 GiB of RAM against 19 GiB of committed guests — already
over-committed, with 4.9 GiB free. node3 also carries one leg of the DNS pair
(`ns2`, `dist2`, `resolver2`) and SeaweedFS, whose 8 GiB cap was raised
deliberately after an OOM (ADR-0018 era). There was no room to grow the VM, and
pushing node3 harder risks the redundancy that lives there. pve has 94.2 GiB
with 28.1 GiB free and already carried the `toolbox2` / `toolbox3` scratch VMs,
which were idle and are retired as part of this change.

## Decision

Relocate `toolbox1` to pve with 16 GiB and `on_boot = true`, moving its stack
to `tf/vm/pve/toolbox1` and its address to `192.168.20.21` — pve exposes
`net20`, not `net50`, and retiring `toolbox2` frees that address. Consolidate
the three toolbox VMs into this one. Serve the editor with **code-server**,
installed from its release `.deb` and managed by an Ansible role, published through Caddy at
`vscode.home.butaco.net` with a Let's Encrypt certificate. Run it as the same
login account that runs the agents. Authenticate with code-server's own argon2
password, stored SOPS-encrypted. Do not expose it to the internet; remote
access goes through `vpngw`.

## Alternatives considered

- **`code tunnel` + vscode.dev.** *Rejected.* The path to a shell holding the
  SOPS AGE key, kubeconfigs and Proxmox/Cloudflare tokens would depend on a
  Microsoft relay and a Microsoft account, with Caddy and a VPN already in
  place locally.
- **`code serve-web` (Microsoft, self-hosted).** *Rejected.* Its licence does
  permit single-user internal hosting, and `--connection-token-file` works
  since July 2024, but the CLI downloads the server build at runtime, so the
  version cannot be pinned. That breaks the pattern every other role follows:
  a version in `defaults/main.yaml`, a marker file, `ops-version_audit.yaml`,
  and Renovate. It also has no supported service mode.
- **A dedicated user without sudo.** *Rejected.* The terminal is the point of
  the service; a separate account would duplicate `~/.claude`, `~/.codex`, SSH
  keys, git config and the AGE key.
- **Terminal-only access (ttyd, or SSH from an iPad client).** *Rejected.* It
  does not provide the file tree, which is half the requirement.
- **Keeping toolbox1 on node3 at 8 GiB.** *Rejected.* Editor, language servers
  and agents share one host, and node3 offers no headroom if 8 GiB proves
  short.
- **Moving toolbox1 to node4.** *Rejected.* node4 is the small always-on host
  chosen for the prd controller (ADR-0021), and its 15.4 GiB could not reach
  16 GiB anyway.

## Consequences

- node3 drops from 19 GiB to 11 GiB committed against 15.4 GiB physical, so its
  over-commit is resolved and the DNS leg regains headroom.
- The move changes the VM's address, so Caddy upstreams and DNS follow. Data on
  the old VM (`~/.claude`, `~/.codex`, SSH keys, AGE key, clones with
  uncommitted work) must be evacuated before it is destroyed.
- Extensions come from Open VSX. The official `Anthropic.claude-code`
  extension is published there, but browser-hosted operation still needs
  confirming per extension.
- Anyone who can log in to the web UI has that account's shell and its sudo
  rights. The password is the only barrier in front of the LAN.
- `toolbox1` joins the `node_exporter` group so the 16 GiB assumption can be
  checked against measurements rather than re-estimated.
