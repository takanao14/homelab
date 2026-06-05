# scripts

Helper scripts for managing the homelab: VM lifecycle, provisioning, secret
sync, and GPU workload switching.

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

| Arg    | Default      | Notes |
|--------|--------------|-------|
| name   | (required)   | Alphanumeric and hyphens only |
| ip     | (required)   | IPv4 without prefix; subnet selects the bridge/gateway |
| node   | `dev`        | `dev` \| `prd` \| `node2` \| `node3` |
| cores  | `4`          | vCPUs |
| memory | `8192`       | MB |
| disk   | `80`         | GB |
| image  | `ubuntu24`   | `ubuntu24` \| `ubuntu24-xrdp` \| `ubuntu26` \| `rocky10` \| `rocky9-xrdp` |

Required env vars (read from `~/.env`): `TF_VM_USERNAME`, `TF_VM_PASSWORD`,
`TF_VM_SSH_PUBLIC_KEY` (per-node overrides like `TF_VM_PASSWORD_DEV` are
supported; falls back to a prompt / `~/.ssh/id_ed25519.pub`).

### `removevm.sh`

Destroys a VM created by `createvm.sh` and removes its Terragrunt directory.

```bash
./removevm.sh <name> [node] [--keep]

./removevm.sh myvm
./removevm.sh myvm prd
./removevm.sh myvm dev --keep   # keep the directory after destroy
```

### `provision.sh`

Provisions an existing VM over SSH in order:

1. Waits for SSH and cloud-init to finish
2. Installs the CLI toolchain (`scripts/install-tools.sh`)
3. Adds `~/.local/bin` to `PATH` and arranges for `~/.env` to be sourced in `~/.bashrc`
4. Installs terminal and fonts (`scripts/install-terminal.sh`, `scripts/install-fonts.sh`)
5. Configures kitty font
6. Fetches env secrets from OpenBao into `~/.env` (`scripts/getenv.sh`)
7. Retrieves kubeconfigs from OpenBao into `~/.kube/` (`scripts/get-kubeconfig.sh`)

Scripts are copied to `/tmp` and run remotely via the `run_remote` helper. The
OpenBao password is entered once and reused across steps.

```bash
./provision.sh <ip> [username]

./provision.sh 192.168.20.50 myuser
```

## Secrets / environment

These three scripts share the same OpenBao auth pattern and can run **locally or
remotely** (over ssh). The password is resolved in order: `BAO_PASSWORD` env var
→ interactive prompt (TTY) → stdin (non-interactive).

Common env vars: `OPENBAO_ADDR` (default `https://openbao.home.butaco.net`),
`BAO_USERNAME`.

### `getenv.sh`

Fetches `secret/provision/env` from OpenBao and writes it to `~/.env`.

```bash
./getenv.sh
```

### `setenv.sh`

Pushes the contents of `~/.env` back into `secret/provision/env`. Defaults to the
`admin` OpenBao user.

```bash
./setenv.sh
```

## Kubernetes

### `gpu-switch.sh`

Switches which single GPU workload runs on the `dev-homelab` cluster by scaling
deployments. Only runs against the `dev-homelab` kube context.

```bash
./gpu-switch.sh <ollama|comfyui|lemonade-server|off>
```

## `scripts/`

### `install-tools.sh`

Thin wrapper that fetches the dotfiles CLI-toolchain installer
(`takanao14/dotfiles`, pinned to the latest `main` commit) and runs it. It
installs the homelab CLI toolchain (kubectl, helm, terragrunt, opentofu,
openbao, sops, age, k9s, kubie, helmfile, cilium, HashiCorp tools …) on Ubuntu
or Rocky; tool versions are pinned and managed by Renovate in dotfiles.

The install mode selects where the tools land:

| Mode | Target | Sudo |
|------|--------|------|
| `local` (default) | `$HOME/.local/bin` (per-user) | no |
| `global` | `/usr/local/bin` (system-wide, for shared / golden-image VMs) | yes |

```bash
./scripts/install-tools.sh            # local (per-user)
./scripts/install-tools.sh global     # system-wide
```

### `install-terminal.sh`

Installs kitty terminal emulator.

### `install-fonts.sh`

Installs UDEV Gothic NFLG fonts.

### `get-kubeconfig.sh`

Retrieves the `dev`/`prd` kubeconfigs from OpenBao into `~/.kube/`.

```bash
./scripts/get-kubeconfig.sh                       # local, interactive
BAO_PASSWORD=xxx ./scripts/get-kubeconfig.sh      # non-interactive
```
