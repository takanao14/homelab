# scripts

Helper scripts for managing the homelab: VM lifecycle, provisioning, secret
sync, and GPU workload switching.

## Layout

```
scripts/
├── createvm.sh / removevm.sh / provision.sh  # VM lifecycle (run directly)
├── gpu-switch.sh                             # k8s GPU workload switch
├── lib/openbao-auth.sh                       # shared OpenBao auth helper
├── install/                                  # CLI toolchain installers (shared with packer/)
│   ├── install-tools.sh / install-terminal.sh / install-fonts.sh
│   └── vendor/                               # vendored dotfiles installers
└── secrets/                                  # OpenBao secret sync
    ├── getenv.sh / get-kubeconfig.sh / get-sops-key.sh   # retrieve
    └── admin/setenv.sh / set-kubeconfig.sh / set-sops-key.sh  # store (privileged)
```

## VM lifecycle

### `createvm.sh`

Generates a Terragrunt config under `tf/vm/<node>/<name>/` and applies it to
create a Proxmox VM. After apply, it waits until SSH on the VM becomes ready.

```bash
./createvm.sh <name> <ip> [node] [cores] [memory_mb] [disk_gb] [image]

# Examples
./createvm.sh myvm 192.168.20.50
./createvm.sh myvm 192.168.20.50 dev 4 4096 80 rocky10
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

### `removevm.sh`

Destroys a VM created by `createvm.sh` and removes its Terragrunt directory.

```bash
./removevm.sh <name> [node] [--keep]

./removevm.sh myvm
./removevm.sh myvm node2
./removevm.sh myvm dev --keep   # keep the directory after destroy
```

### `provision.sh`

Provisions an existing VM over SSH in order:

1. Waits for SSH and cloud-init to finish
2. Installs the CLI toolchain (`install/install-tools.sh`)
3. Adds `~/.local/bin` to `PATH` and arranges for `~/.env` to be sourced in `~/.bashrc`
4. Installs terminal and fonts (`install/install-terminal.sh`, `install/install-fonts.sh`)
5. Configures kitty font
6. Fetches env secrets from OpenBao into `~/.env` (`secrets/getenv.sh`)
7. Retrieves kubeconfigs from OpenBao into `~/.kube/` (`secrets/get-kubeconfig.sh`)

Scripts are copied to `/tmp` and run remotely via the `run_remote` helper, which
mirrors each script's path relative to `scripts/` under `/tmp` so it resolves its
siblings the same way as locally. The vendored installers (`install/vendor/`) are
copied to `/tmp/install/vendor/` so the `install-*.sh` wrappers run local copies
instead of downloading from GitHub. The
OpenBao credentials are reused across steps. When `BAO_TOKEN` is set, it is
forwarded to the remote scripts over stdin; otherwise the password is entered
once and reused.

```bash
./provision.sh <ip> [username]

./provision.sh 192.168.20.50 myuser
```

## Secrets / environment

These OpenBao scripts share the same auth pattern and can run **locally or
remotely** (over ssh). Authentication is resolved in order: `BAO_TOKEN` env var
→ `BAO_PASSWORD` env var → interactive prompt (TTY) → stdin (non-interactive).
When `BAO_TOKEN` is set, userpass login is skipped and the token is used as-is.
An invalid or insufficient token fails the requested operation; unset
`BAO_TOKEN` to use password authentication instead.

Common env vars: `OPENBAO_ADDR` (default `https://openbao.home.butaco.net`),
`BAO_USERNAME`, `BAO_TOKEN`, `BAO_PASSWORD`.

### `getenv.sh`

Fetches `secret/provision/env` from OpenBao and writes it to `~/.env`.
Updates are written via a temporary file and moved into place only after a
successful fetch. Values are double-quoted so `$VAR` and `${VAR}` references
expand when sourced by Bash. Command substitutions are rejected.

```bash
./secrets/getenv.sh
BAO_TOKEN=xxx ./secrets/getenv.sh
```

### `setenv.sh`

Pushes the contents of `~/.env` back into `secret/provision/env`. Defaults to the
`admin` OpenBao user. Values are parsed without sourcing the file, so shell
variables such as `$HOME` remain literal and command substitutions are not run.

```bash
./secrets/admin/setenv.sh
BAO_TOKEN=xxx ./secrets/admin/setenv.sh
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

### `install-tools.sh`

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
./install/install-tools.sh            # local (per-user)
./install/install-tools.sh global     # system-wide
```

### `install-terminal.sh`

Installs the kitty terminal emulator. Like `install-tools.sh`, the install mode
selects where kitty lands:

| Mode | Target | Sudo |
|------|--------|------|
| `local` (default) | `$HOME/.local/kitty.app` (per-user) | no |
| `global` | `/usr/local/kitty.app` (system-wide, for shared / golden-image VMs) | yes |

```bash
./install/install-terminal.sh            # local (per-user)
./install/install-terminal.sh global     # system-wide
```

### `install-fonts.sh`

Installs the UDEV Gothic NF font. Like `install-tools.sh`, the install mode
selects where the font lands:

| Mode | Target | Sudo |
|------|--------|------|
| `local` (default) | `$HOME/.local/share/fonts` (per-user) | no |
| `global` | `/usr/local/share/fonts` (system-wide, for shared / golden-image VMs) | yes |

```bash
./install/install-fonts.sh            # local (per-user)
./install/install-fonts.sh global     # system-wide
```

### `vendor/`

Local copies of the dotfiles installer scripts that `install-tools.sh`,
`install-terminal.sh`, and `install-fonts.sh` run. Vendoring them means
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
