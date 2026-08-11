# scripts

Helper scripts for managing the homelab: VM lifecycle, provisioning, secret
sync, and GPU workload switching.

## Layout

```
scripts/
├── create-vm.sh / remove-vm.sh / provision.sh  # VM lifecycle (run directly)
├── gpu-switch.sh                             # k8s GPU workload switch
├── check-image-refs.sh                       # CI: image filename map consistency check
├── grafana-mcp.sh                            # Grafana MCP server launcher (stdio)
├── grafana-mcp-token.sh                      # issue Grafana MCP service-account token
├── netbox-mcp.sh                             # NetBox MCP server launcher (stdio)
├── netbox-mcp-token.sh                       # generate the managed NetBox MCP v2 token
├── lib/openbao-auth.sh                       # shared OpenBao auth helper
├── lib/mcp-launcher.sh                       # shared MCP launcher plumbing (SOPS env, container runtime)
├── install/                                  # CLI toolchain installers (shared with packer/)
│   ├── tools.sh / terminal.sh / fonts.sh
│   └── vendor/                               # vendored dotfiles installers
└── secrets/                                  # OpenBao secret sync
    ├── get-env.sh / get-kubeconfig.sh / get-sops-key.sh   # retrieve
    └── admin/set-env.sh / set-kubeconfig.sh / set-sops-key.sh  # store (privileged)
```

## VM lifecycle

### `create-vm.sh`

Generates a Terragrunt config under `tf/vm/<node>/<name>/`. It does not run
`plan`, `apply`, or wait for SSH; those steps remain separate so a failed
operation can be inspected and resumed directly with Terragrunt.

```bash
./create-vm.sh <name> <ip> [node] [cores] [memory_mb] [disk_gb] [image]

# Examples
./create-vm.sh myvm 192.168.20.50
./create-vm.sh myvm 192.168.20.50 pve 4 4096 80 rocky10
```

| Arg    | Default      | Notes                                                      |
|--------|--------------|------------------------------------------------------------|
| name   | (required)   | Alphanumeric and hyphens only                              |
| ip     | (required)   | IPv4 without prefix; subnet selects the bridge/gateway     |
| node   | `pve`        | `pve` \| `node2` \| `node3` \| `node4`                    |
| cores  | `4`          | vCPUs                                                      |
| memory | `8192`       | MB                                                         |
| disk   | `80`         | GB                                                         |
| image  | `ubuntu24`   | `ubuntu24[-xrdp]`, `rocky10`, `rocky9[-xrdp]`, `debian13` |

The XRDP images are available only on `pve`.

The script uses `TF_VM_USERNAME`, `TF_VM_PASSWORD`, and
`TF_VM_SSH_PUBLIC_KEY` for the VM credentials. When they are unset, the
username defaults to the current user, the password is read from an
interactive hidden prompt, and the public key defaults to
`~/.ssh/id_ed25519.pub`. The public-key file must exist. For non-interactive
use, set `TF_VM_PASSWORD` explicitly. The resolved values are written to a
local `.envrc` in the generated VM directory and reused on subsequent runs.
The file is created with mode `600` and excluded by a colocated `.gitignore`;
it contains the VM password in plaintext and must remain local. Run
`direnv allow` once after generation.

Re-running the command with the same arguments succeeds without changing the
file. If the existing file differs, the script prints a diff and leaves it
unchanged.

Review and create the VM explicitly:

```bash
cd ../tf/vm/pve/myvm
direnv exec . terragrunt plan
direnv exec . terragrunt apply

# Run from scripts/ after the VM has been created.
./provision.sh 192.168.20.50
```

Terragrunt obtains `TF_VM_USERNAME`, `TF_VM_PASSWORD`, and
`TF_VM_SSH_PUBLIC_KEY` from the generated VM directory's `.envrc`. It calls
`source_up` first so the node and shared Terraform environment is preserved.

### `remove-vm.sh`

Destroys a VM created by `create-vm.sh` and removes its Terragrunt directory.

```bash
./remove-vm.sh <name> [node] [--keep]

./remove-vm.sh myvm
./remove-vm.sh myvm node2
./remove-vm.sh myvm pve --keep   # keep the directory after destroy
```

### `provision.sh`

Provisions a VM in order (over SSH by default, or in place with `--local`):

1. Waits for SSH and cloud-init to finish
2. Installs system-package prerequisites (`install/packages.sh`), or verifies
   them without sudo in `--local` mode
3. Installs the CLI toolchain (`install/tools.sh`)
4. Adds `~/.local/bin` to `PATH` and arranges for `~/.env` to be sourced in `~/.bashrc`
5. Installs terminal and fonts (`install/terminal.sh`, `install/fonts.sh`)
6. Configures kitty font
7. Fetches env secrets from OpenBao into `~/.env` (`secrets/get-env.sh`)
8. Retrieves kubeconfigs from OpenBao into `~/.kube/` (`secrets/get-kubeconfig.sh`)

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
The cloud-init wait checks the standard `/var/lib/cloud/instance/boot-finished`
marker and `cloud-init status` before provisioning continues. The default wait
timeout is 600 seconds and can be overridden with `CLOUD_INIT_WAIT_TIMEOUT`.

```bash
./provision.sh <ip> [username]      # remote: push to the VM at <ip> over SSH
./provision.sh --local [username]    # local: provision this machine directly

./provision.sh 192.168.20.50 myuser
CLOUD_INIT_WAIT_TIMEOUT=900 ./provision.sh 192.168.20.50 myuser
./provision.sh --local               # run on the target Linux box as that user
```

In `--local` mode there is no SSH hop: the SSH-wait, the `tar`-over-`ssh`
staging, and the `/tmp` cleanup `trap` are skipped, and each step runs the real
script under `scripts/` (resolving its siblings the same way) or executes the
shell snippet directly. It must run **on the target Linux box** as the user being
provisioned (no `su`), so `[username]` is optional and, if given, must match
`$USER`. Supported distributions are Ubuntu, Debian, and Rocky Linux. The
system-package step runs with `TOOL_SKIP_SYSTEM_PACKAGES=1`, so it never invokes
sudo and fails fast when the required packages were not baked into the image.
The remaining install steps stay in per-user (`local`) mode, landing tools under
`$HOME/.local`.

### `check-image-refs.sh`

Cross-checks the image-filename maps duplicated across `create-vm.sh`,
`packer/build.sh` and `packer/push.sh` against the definitions in
`tf/customimage/images.hcl` (and `tf/cloudimage/images.hcl`). Run by CI on
changes to any of those files (`.github/workflows/image-refs.yaml`); run it
manually after adding or renaming an image target.

```bash
./check-image-refs.sh
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

Retrieves the `prd`/`sandbox` kubeconfigs from OpenBao into `~/.kube/`. Existing
files are replaced only after both kubeconfigs are fetched successfully.

```bash
./secrets/get-kubeconfig.sh                       # local, interactive
BAO_TOKEN=xxx ./secrets/get-kubeconfig.sh         # token auth
BAO_PASSWORD=xxx ./secrets/get-kubeconfig.sh      # non-interactive
```

### `set-kubeconfig.sh`

Stores `~/.kube/prd.yaml` and `~/.kube/sandbox.yaml` in OpenBao at
`secret/kubeconfig/prd` and `secret/kubeconfig/sandbox`. Defaults to the `admin`
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
decrypt the repo's `*.sops.yaml` and `*.sops.env` files; it is intentionally
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

Switches which single GPU workload runs on the `prd-homelab` cluster by scaling
deployments. Only runs against the `prd-homelab` kube context.

```bash
./gpu-switch.sh <workload|off|status>   # e.g. ./gpu-switch.sh ollama
```

`status` answers "which workload currently holds the GPU":

```console
$ ./gpu-switch.sh status
  comfyui            stopped
  lemonade-server    stopped
* ollama             running (1/1)
  vllm               stopped
```

`starting (0/1)` means the deployment is scaled up but its pod is not ready yet
— on a single GPU that also covers a pod stuck `Pending` because the device is
still held. Replica count is runtime state and is deliberately not tracked in
Git (the Argo CD `Application`s ignore `/spec/replicas`), so the cluster is the
only place to ask.

Targets are **discovered from the cluster** by the `homelab/gpu-switchable`
label, not hardcoded in the script, so a GPU workload becomes switchable by
carrying that label on its `Deployment` — which it already needs for the Argo CD
health checks (ADR-0027). Run with an unknown name to list what is available.

## Grafana MCP

Scripts backing the Grafana MCP server registered in the repo-root `.mcp.json`.
The server lets an MCP client (Claude Code, Codex, Cursor, …) query Grafana
(PromQL/LogQL and dashboards) against `https://grafana.prd.butaco.net`.

### `grafana-mcp.sh`

Launcher invoked by the MCP client over stdio. It selects a container runtime
per OS — `docker` on macOS when OrbStack is installed, `podman` otherwise — and
runs the `grafana/mcp-grafana` image with `-i` (no TTY). You normally don't run
this by hand; the client starts it.

The image is the vendor's own, pinned to a release tag. Docker's curated
`mcp/grafana` mirror publishes only a `latest` tag, so it could be neither
pinned nor tracked by Renovate. Note that the binary reports its version as
`(devel)` regardless — upstream does not inject version info into the Docker
build, so the image tag is the only reliable indicator of what is running.

The launcher exposes only `search`, `datasource`, `prometheus`, `loki`,
`dashboard`, and `navigation` tools by default. Alert-management tools are
excluded from the default allowlist. It also passes
`--disable-write` and limits Loki results to 20 lines. This keeps the tool
schema and responses small enough for local models while enforcing read-only
operation independently of Grafana permissions.

Credentials are **self-resolving**: if `GRAFANA_SERVICE_ACCOUNT_TOKEN` is already
exported (e.g. Claude Code launched under direnv) it is used as-is; otherwise the
script decrypts `.env/secrets.sops.env` via `sops` itself, deriving the repo root
from its own path. This means the same launcher works from any client regardless
of cwd or whether direnv has loaded — clients only need to point at this script,
never embed the token. `GRAFANA_URL` defaults to the prd Grafana.

Other clients just reference the absolute path, e.g. Codex (`~/.codex/config.toml`):

```toml
[mcp_servers.grafana]
command = "/Users/takanao/lab/homelab/scripts/grafana-mcp.sh"
startup_timeout_ms = 60000   # first run pulls the grafana/mcp-grafana image
```

### Goose workstation setup

Goose is the default local-LLM agent for this repository. Run the agent on the
workstation and keep only Ollama in the cluster. This gives the agent direct
access to the checked-out repository and preserves interactive approval while
avoiding another long-running in-cluster agent runtime.

The validated baseline is:

| Setting | Value |
|---------|-------|
| Agent | Goose |
| Provider | Ollama |
| Endpoint | `https://ollama.prd.butaco.net` |
| Model | `qwen3:14b` |
| Context | `32768` (configured by the Ollama deployment) |
| Mode | `approve` |
| Maximum turns | `10` |
| Telemetry | disabled |
| Grafana MCP | local stdio launcher, enabled |

Install and configure the client interactively:

```bash
brew install block-goose-cli
goose configure
```

Select the Ollama provider and set the endpoint/model above. Add a
**Command-line Extension** named `grafana` with:

```text
/absolute/path/to/homelab/scripts/grafana-mcp.sh
```

Use a 300-second extension timeout and do not add token environment variables
to the Goose configuration. The launcher resolves the encrypted token itself.
Keep `GOOSE_MODE=approve` for interactive use. A one-shot `GOOSE_MODE=auto`
override is appropriate only for a narrowly scoped task whose enabled MCP
servers are read-only.

Smoke-test the complete tool loop from Goose:

```text
Use only the Grafana MCP. List each Grafana data source name and type.
Do not modify Grafana or any files.
```

The Grafana extension is healthy when Goose selects `list_datasources` and
returns the Prometheus and Loki data sources. A broader read-only check can
search for a dashboard, call `get_dashboard_summary`, and then call
`get_dashboard_panel_queries` using the returned UID.

If the MCP server does not start, run `bash -n scripts/grafana-mcp.sh`, confirm
that Docker or Podman is running, and check that `sops` can access the AGE key.
If model requests end at a fixed duration, inspect the Ollama HTTPRoute request
and backend-request timeouts before changing Goose's extension timeout.

| Env var | Default | Notes |
|---------|---------|-------|
| `GRAFANA_MCP_RUNTIME` | `docker` on macOS with OrbStack, otherwise `podman` | Force a runtime |
| `GRAFANA_MCP_VERSION` | `1.1.0` | Pinned image tag; Renovate bumps this in place |
| `GRAFANA_MCP_IMAGE`   | `docker.io/grafana/mcp-grafana:$GRAFANA_MCP_VERSION` | Full reference override for testing |
| `GRAFANA_MCP_ENABLED_TOOLS` | `search,datasource,prometheus,loki,dashboard,navigation` | Comma-separated tool categories |
| `GRAFANA_MCP_DISABLE_WRITE` | `true` | Set to `false` only for an explicitly approved write workflow |
| `GRAFANA_MCP_MAX_LOKI_LOG_LIMIT` | `20` | Maximum log lines returned by a Loki query |

On macOS, the Podman fallback expects a working Podman machine. Prefer the
official Podman macOS installer; Homebrew builds can miss VM helper binaries
such as `krunkit`. After installing Podman, initialize the machine with
`podman machine init --now`.

### `grafana-mcp-token.sh`

Idempotently creates the `mcp-grafana` service account (Viewer role) and issues
a token, printing it to stdout as a `GRAFANA_SERVICE_ACCOUNT_TOKEN="..."`
line (logs go to stderr). Admin auth is taken from `GRAFANA_ADMIN_USER` /
`GRAFANA_ADMIN_PASSWORD`, or read from the in-cluster `grafana-admin` secret via
`kubectl --context "${GRAFANA_KUBE_CONTEXT:-prd-homelab}"` so it never depends on
the currently selected context. Requires `curl` and `jq` (and `kubectl` for the
secret path).

```bash
# Print the token line
./grafana-mcp-token.sh

# Store it: paste the printed line over the existing assignment
sops edit ../.env/secrets.sops.env
direnv allow
```

`../.env/secrets.sops.env` is already encrypted, so the append-then-encrypt
shortcut (`>> file` followed by `sops --encrypt --in-place file`) does **not**
work — SOPS refuses with exit 203 because the file already carries its `sops_*`
metadata. `sops edit` is the only flow that survives a rotation.

| Env var | Default | Notes |
|---------|---------|-------|
| `GRAFANA_URL`           | `https://grafana.prd.butaco.net` | Target Grafana |
| `GRAFANA_KUBE_CONTEXT`  | `prd-homelab`                    | Context for the admin secret |
| `GRAFANA_MCP_SA_NAME`   | `mcp-grafana`                    | Service account name |
| `GRAFANA_MCP_SA_ROLE`   | `Viewer`                         | Service account role |
| `GRAFANA_MCP_TOKEN_NAME`| `mcp-grafana-<timestamp>`        | Token name |

Re-running issues a **new** token while reusing the existing service account;
revoke unused tokens in the Grafana UI or via the API.

## NetBox MCP

`netbox-mcp.sh` runs the official
[`netboxlabs/netbox-mcp-server`](https://github.com/netboxlabs/netbox-mcp-server)
container over stdio. The image is pinned to `1.2.1`, and the server exposes
read-only NetBox query tools.

The image carries the upstream `uv.lock`, so transitive dependencies are pinned
alongside the release tag. (A `uvx --from git+…@tag` install pins only the tag
and re-resolves everything below it — the two paths resolved different FastMCP
versions in practice.) Upstream publishes multi-arch images signed with cosign.

Runtime selection and SOPS credential resolution are shared with
`grafana-mcp.sh` through `lib/mcp-launcher.sh`, so both launchers behave
identically: `NETBOX_URL` and `NETBOX_TOKEN` are taken from the environment when
present, otherwise decrypted from `.env/secrets.sops.env`. MCP client
configuration therefore contains no API token, and only variable *names* are
passed to the container (`-e NAME`), keeping values off the command line.

Upstream's README states that containers require `TRANSPORT=http` because stdio
"doesn't work" there. That applies to detached containers; `docker run -i`
without a TTY keeps stdio framing intact, which is what this launcher uses and
what the Grafana launcher has always done.

A container runtime must be running — on macOS this is OrbStack (or Podman).
This is the one operational cost of the container over `uvx`: if the runtime is
cold, the server fails to start until it is up.

Generate the desired v2 token, then store it by editing the existing encrypted
environment file:

```bash
./scripts/netbox-mcp-token.sh
sops edit .env/secrets.sops.env
direnv allow
```

Add this assignment inside the decrypted editor buffer:

```bash
NETBOX_TOKEN="<read-only-api-token>"
```

The `netbox` Ansible role reads this environment variable, creates the
`mcp-netbox` user and `mcp-readers` group, assigns infrastructure-only `view`
permissions, and creates the matching token with API writes disabled. NetBox
keeps only an HMAC of a v2 token, which is why the value is generated here
rather than read back from the API. Preview and apply it with:

```bash
cd ansible
ansible-playbook playbooks/netbox.yaml --check --diff --tags netbox

# The operator performs the live change after reviewing the check output.
ansible-playbook playbooks/netbox.yaml --tags netbox
```

The identity steps run through `manage.py` and are therefore skipped under
`--check`, like the other `manage.py` tasks in the role; the check run confirms
the rest of the play. Rotating the token is the same three commands as first
setup — generate, `sops edit`, re-run the play — and the role replaces the old
token row. See [`ansible/roles/netbox/README.md`](../ansible/roles/netbox/README.md)
for the permission scope and the variables that adjust it.

Do not commit a plaintext token or pass it in `.codex/config.toml` or
`.mcp.json`. Start a new client session after changing MCP configuration. The
first launch pulls the image, so it needs network access; later launches reuse
the local image.

| Env var | Default | Notes |
|---------|---------|-------|
| `NETBOX_URL` | `https://netbox-ui.home.butaco.net/` | Target NetBox instance |
| `NETBOX_TOKEN` | none | Required read-only API token |
| `VERIFY_SSL` | `true` | Keep TLS verification enabled |
| `ENABLE_PLUGIN_DISCOVERY` | `false` | Enable only when NetBox plugin models are needed |
| `NETBOX_MCP_VERSION` | `1.2.1` | Pinned image tag; Renovate bumps this in place |
| `NETBOX_MCP_IMAGE` | `docker.io/netboxlabs/netbox-mcp-server:$NETBOX_MCP_VERSION` | Full reference override for testing |
| `NETBOX_MCP_RUNTIME` | auto (OrbStack `docker`, else `podman`) | Force a container runtime |

Renovate tracks the Docker Hub tag through the `# renovate:` comment above
`netbox_mcp_version`, matched by the `scripts/*.sh` custom manager in
[`renovate.json`](../renovate.json). Updates land as a regular PR; they are not
automerged, since the automerge rule covers only `k8s/`, `ansible/`, and `tf/`.
The Grafana launcher is tracked the same way, against `grafana/mcp-grafana`.

## `install/`

### `packages.sh`

Thin wrapper that runs the **vendored** dotfiles system-package installer
(`vendor/run_onchange_linux0_package.sh`, see [`vendor/`](#vendor)). It owns the
privileged package-manager operations and must run before `tools.sh` and
`fonts.sh`.

Set `TOOL_SKIP_SYSTEM_PACKAGES=1` to perform a no-sudo preflight instead of
installing packages. This is used by `provision.sh --local`, where a golden image
is expected to provide the prerequisites already.

```bash
./install/packages.sh                              # install packages via sudo
TOOL_SKIP_SYSTEM_PACKAGES=1 ./install/packages.sh  # no-sudo preflight
./install/packages.sh global                       # system-wide version cache
```

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

Local copies of the dotfiles installer scripts that `packages.sh`, `tools.sh`,
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
