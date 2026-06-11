# scripts

Helper scripts for managing the homelab: VM lifecycle, provisioning, secret
sync, and GPU workload switching.

## Layout

```
scripts/
├── create-vm.sh / remove-vm.sh / provision.sh  # VM lifecycle (run directly)
├── gpu-switch.sh                             # k8s GPU workload switch
├── lib/openbao-auth.sh                       # shared OpenBao auth helper
├── install/                                  # CLI toolchain installers (shared with packer/)
│   ├── tools.sh / terminal.sh / fonts.sh
│   └── vendor/                               # vendored dotfiles installers
└── secrets/                                  # OpenBao secret sync
    ├── get-env.sh / get-kubeconfig.sh / get-sops-key.sh   # retrieve
    └── admin/set-env.sh / set-kubeconfig.sh / set-sops-key.sh  # store (privileged)
```

## VM lifecycle

### `create-vm.sh`

Generates a Terragrunt config under `tf/vm/<node>/<name>/` and applies it to
create a Proxmox VM. After apply, it waits until SSH on the VM becomes ready.

```bash
./create-vm.sh <name> <ip> [node] [cores] [memory_mb] [disk_gb] [image]

# Examples
./create-vm.sh myvm 192.168.20.50
./create-vm.sh myvm 192.168.20.50 dev 4 4096 80 rocky10
```

| Arg    | Default      | Notes                                                      |
|--------|--------------|------------------------------------------------------------|
| name   | (required)   | Alphanumeric and hyphens only                              |
| ip     | (required)   | IPv4 without prefix; subnet selects the bridge/gateway     |
| node   | `dev`        | `dev` \| `node2` \| `node3`                                |
| cores  | `4`          | vCPUs                                                      |
| memory | `8192`       | MB                                                         |
| disk   | `80`         | GB                                                         |
| image  | `ubuntu24`   | `ubuntu24` \| `ubuntu24-xrdp` \| `rocky10` \| `rocky9-xrdp` |

Required env vars (read from `~/.env`): `TF_VM_USERNAME`, `TF_VM_PASSWORD`,
`TF_VM_SSH_PUBLIC_KEY` (per-node overrides like `TF_VM_PASSWORD_DEV` and
`TF_VM_SSH_PUBLIC_KEY_NODE2` are supported; falls back to a prompt /
`~/.ssh/id_ed25519.pub`).

### `remove-vm.sh`

Destroys a VM created by `create-vm.sh` and removes its Terragrunt directory.

```bash
./remove-vm.sh <name> [node] [--keep]

./remove-vm.sh myvm
./remove-vm.sh myvm node2
./remove-vm.sh myvm dev --keep   # keep the directory after destroy
```

### `provision.sh`

Provisions a VM in order (over SSH by default, or in place with `--local`):

1. Waits for SSH and cloud-init to finish
2. Installs the CLI toolchain (`install/tools.sh`)
3. Adds `~/.local/bin` to `PATH` and arranges for `~/.env` to be sourced in `~/.bashrc`
4. Installs terminal and fonts (`install/terminal.sh`, `install/fonts.sh`)
5. Configures kitty font
6. Fetches env secrets from OpenBao into `~/.env` (`secrets/get-env.sh`)
7. Retrieves kubeconfigs from OpenBao into `~/.kube/` (`secrets/get-kubeconfig.sh`)

All scripts are staged once under `/tmp/homelab-provision/` in a single
`tar`-over-`ssh` step (`stage_scripts`), preserving each script's path relative
to `scripts/` so it resolves its siblings the same way as locally (e.g.
`install/tools.sh` finds `install/vendor/`). Because the vendored installers ride
along, the `install/*.sh` wrappers run those local copies instead of downloading
from GitHub. Only the `secrets/get-*` readers are staged; the privileged
`admin/set-*` scripts are never copied to the VM. The staged directory is removed
on exit (success or failure) via a `trap`.

Each step then runs through the `run_remote` helper, which is a single `ssh`
invocation, so a piped credential reaches the script intact. The OpenBao
credential is reused across steps: when `BAO_TOKEN` is set it is forwarded to the
remote scripts over stdin; otherwise the password is entered once and reused.

```bash
./provision.sh <ip> [username]      # remote: push to the VM at <ip> over SSH
./provision.sh --local [username]    # local: provision this machine directly

./provision.sh 192.168.20.50 myuser
./provision.sh --local               # run on the target Linux box as that user
```

In `--local` mode there is no SSH hop: the SSH-wait, the `tar`-over-`ssh`
staging, and the `/tmp` cleanup `trap` are skipped, and each step runs the real
script under `scripts/` (resolving its siblings the same way) or executes the
shell snippet directly. It must run **on the target Linux box** as the user being
provisioned (no `su`), so `[username]` is optional and, if given, must match
`$USER`. Supported distributions are Ubuntu, Debian, and Rocky Linux. The
install steps stay in per-user (`local`) mode, landing tools under `$HOME/.local`.

## Secrets / environment

These OpenBao scripts share the same auth pattern and can run **locally or
remotely** (over ssh). Authentication is resolved in order: `BAO_TOKEN` env var
→ `BAO_PASSWORD` env var → interactive prompt (TTY) → stdin (non-interactive).
When `BAO_TOKEN` is set, userpass login is skipped and the token is used as-is.
An invalid or insufficient token fails the requested operation; unset
`BAO_TOKEN` to use password authentication instead.

Common env vars: `OPENBAO_ADDR` (default `https://openbao.home.butaco.net`),
`BAO_USERNAME`, `BAO_TOKEN`, `BAO_PASSWORD`.

### `get-env.sh`

Fetches `secret/provision/env` from OpenBao and writes it to `~/.env`.
Updates are written via a temporary file and moved into place only after a
successful fetch. Values are double-quoted so `$VAR` and `${VAR}` references
expand when sourced by Bash. Command substitutions are rejected.

```bash
./secrets/get-env.sh
BAO_TOKEN=xxx ./secrets/get-env.sh
```

### `set-env.sh`

Pushes the contents of `~/.env` back into `secret/provision/env`. Defaults to the
`admin` OpenBao user. Values are parsed without sourcing the file, so shell
variables such as `$HOME` remain literal and command substitutions are not run.

```bash
./secrets/admin/set-env.sh
BAO_TOKEN=xxx ./secrets/admin/set-env.sh
```

### `get-kubeconfig.sh`

Retrieves the `dev`/`prd` kubeconfigs from OpenBao into `~/.kube/`. Existing
files are replaced only after both kubeconfigs are fetched successfully.

```bash
./secrets/get-kubeconfig.sh                       # local, interactive
BAO_TOKEN=xxx ./secrets/get-kubeconfig.sh         # token auth
BAO_PASSWORD=xxx ./secrets/get-kubeconfig.sh      # non-interactive
```

### `set-kubeconfig.sh`

Stores `~/.kube/dev.yaml` and `~/.kube/prd.yaml` in OpenBao at
`secret/kubeconfig/dev` and `secret/kubeconfig/prd`. Defaults to the `admin`
OpenBao user and validates both files before writing either secret.

```bash
./secrets/admin/set-kubeconfig.sh
BAO_TOKEN=xxx ./secrets/admin/set-kubeconfig.sh
BAO_PASSWORD=xxx ./secrets/admin/set-kubeconfig.sh
```

### `get-sops-key.sh`

Retrieves the SOPS age private key from OpenBao (`secret/sops/age`) into
`~/.config/sops/age/keys.txt` (override with `SOPS_AGE_KEY_FILE`). The file is
written via a temporary file and moved into place only after the value is
fetched and validated as an age private key. This is the bootstrap key used to
decrypt the repo's `*.sops.yaml` and `*.enc.env` files; it is intentionally
**not** part of the default `provision.sh` flow, so run it explicitly only where
SOPS decryption is needed.

```bash
./secrets/get-sops-key.sh                       # local, interactive
BAO_TOKEN=xxx ./secrets/get-sops-key.sh         # token auth
BAO_PASSWORD=xxx ./secrets/get-sops-key.sh      # non-interactive
```

### `set-sops-key.sh`

Stores `~/.config/sops/age/keys.txt` (override with `SOPS_AGE_KEY_FILE`) in
OpenBao at `secret/sops/age`. Defaults to the `admin` OpenBao user and validates
the file before writing.

```bash
./secrets/admin/set-sops-key.sh
BAO_TOKEN=xxx ./secrets/admin/set-sops-key.sh
BAO_PASSWORD=xxx ./secrets/admin/set-sops-key.sh
```

## Kubernetes

### `gpu-switch.sh`

Switches which single GPU workload runs on the `dev-homelab` cluster by scaling
deployments. Only runs against the `dev-homelab` kube context.

```bash
./gpu-switch.sh <ollama|comfyui|lemonade-server|off>
```

## `install/`

### `tools.sh`

Thin wrapper that runs the **vendored** dotfiles CLI-toolchain installer
(`vendor/run_onchange_linux1_tool.sh`, see [`vendor/`](#vendor)). It installs the
homelab CLI toolchain (kubectl, helm, terragrunt, opentofu, openbao, sops, age,
k9s, kubie, helmfile, cilium, HashiCorp tools …) on Ubuntu or Rocky; tool
versions are pinned and managed by Renovate in dotfiles.

The install mode selects where the tools land:

| Mode | Target | Sudo |
|------|--------|------|
| `local` (default) | `$HOME/.local/bin` (per-user) | no |
| `global` | `/usr/local/bin` (system-wide, for shared / golden-image VMs) | yes |

```bash
./install/tools.sh            # local (per-user)
./install/tools.sh global     # system-wide
```

### `terminal.sh`

Installs the kitty terminal emulator. Like `tools.sh`, the install mode
selects where kitty lands:

| Mode | Target | Sudo |
|------|--------|------|
| `local` (default) | `$HOME/.local/kitty.app` (per-user) | no |
| `global` | `/usr/local/kitty.app` (system-wide, for shared / golden-image VMs) | yes |

```bash
./install/terminal.sh            # local (per-user)
./install/terminal.sh global     # system-wide
```

### `fonts.sh`

Installs the UDEV Gothic NF font. Like `tools.sh`, the install mode
selects where the font lands:

| Mode | Target | Sudo |
|------|--------|------|
| `local` (default) | `$HOME/.local/share/fonts` (per-user) | no |
| `global` | `/usr/local/share/fonts` (system-wide, for shared / golden-image VMs) | yes |

```bash
./install/fonts.sh            # local (per-user)
./install/fonts.sh global     # system-wide
```

### `vendor/`

Local copies of the dotfiles installer scripts that `tools.sh`,
`terminal.sh`, and `fonts.sh` run. Vendoring them means
provisioning no longer fetches them from GitHub at runtime, so it does not depend
on the GitHub API rate limit or `raw.githubusercontent.com` being reachable. The
pinned source commit is recorded in `vendor/REVISION`.

Do not edit the `run_onchange_*.sh` files by hand — they are kept in sync with
`takanao14/dotfiles` by `vendor/sync.sh`:

```bash
./install/vendor/sync.sh           # refresh to the latest dotfiles main
REF=<sha|tag> ./install/vendor/sync.sh   # pin to a specific ref
./install/vendor/sync.sh --check   # CI: fail if the vendored copies have drifted
```
