# scripts

Homelab VM lifecycle, provisioning, secret sync, checks, and integrations.

## Layout

```
scripts/
├── create-vm.sh / remove-vm.sh / provision.sh  # VM lifecycle (run directly)
├── gpu-switch.sh                             # k8s GPU workload switch
├── adr-lint.py                               # ADR metadata, index, and link validation
├── check-image-refs.sh                       # CI: image filename map consistency check
├── check-proxy-routes.sh                     # CI: Caddy sites <-> Homepage dashboard drift check
├── check-openbao-secrets.sh                  # declared secret paths <-> OpenBao KV drift check (local only)
├── grafana-mcp.sh                            # Grafana MCP server launcher (stdio)
├── grafana-mcp-token.sh                      # issue Grafana MCP service-account token
├── netbox-mcp.sh                             # NetBox MCP server launcher (stdio)
├── netbox-mcp-token.sh                       # generate the managed NetBox MCP v2 token
├── mcp-http-headers.py                       # HTTP MCP bearer headers from SOPS
├── lib/openbao-auth.sh                       # shared OpenBao auth helper
├── lib/mcp-launcher.sh                       # shared MCP launcher plumbing (SOPS env, container runtime)
├── install/                                  # CLI toolchain installers (shared with packer/)
│   ├── packages.sh / tools.sh / terminal.sh / fonts.sh
│   └── vendor/                               # vendored dotfiles installers
└── secrets/                                  # OpenBao secret sync
    ├── get-env.sh / get-kubeconfig.sh / get-sops-key.sh   # retrieve
    └── admin/                                # store workstation secrets and seed MCP tokens
```

`install/vendor/REVISION` pins the dotfiles source SHA. Edit synchronized files
in dotfiles, then update them through [vendor sync](#vendor).

## VM lifecycle

### `create-vm.sh`

Generates a Terragrunt config under `tf/vm/<node>/<name>/`. It does not run
`plan`, `apply`, or wait for SSH; those steps remain separate so the
operator can inspect and resume a failed operation directly with Terragrunt.

```bash
./create-vm.sh <name> <ip> [node] [cores] [memory_mb] [disk_gb] [image]

# Examples
./create-vm.sh myvm 192.168.20.50
./create-vm.sh myvm 192.168.20.50 pve 4 4096 80 rocky10-base
```

| Arg    | Default      | Notes                                                      |
|--------|--------------|------------------------------------------------------------|
| name   | (required)   | Alphanumeric and hyphens only                              |
| ip     | (required)   | IPv4 without prefix; subnet selects the bridge/gateway     |
| node   | `pve`        | `pve` \| `node2` \| `node3` \| `node4`                    |
| cores  | `4`          | vCPUs                                                      |
| memory | `8192`       | MB                                                         |
| disk   | `80`         | GB                                                         |
| image  | `ubuntu24-base` | `ubuntu24-{base,tool,desktop}`, `rocky10-base`, `rocky9-{base,desktop}`, `debian13-base` |

Image roles and build targets are described in [packer/README.md](../packer/README.md#image-roles).

The image stacks download `tool` and `desktop` images only on `pve`; create
VMs using those images there.

VM credentials use `TF_VM_USERNAME`, `TF_VM_PASSWORD`, and
`TF_VM_SSH_PUBLIC_KEY`. Defaults are the current user, a hidden password prompt,
and `~/.ssh/id_ed25519.pub`; non-interactive runs must set the password. The
script saves resolved credentials in a mode-600, gitignored `.envrc` for reuse.
This file contains plaintext credentials. Keep it local and run `direnv allow` once.

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

The generated `.envrc` calls `source_up` to retain node/shared Terraform settings.

### `remove-vm.sh`

Destroys a VM created by `create-vm.sh` and removes its Terragrunt directory.

```bash
./remove-vm.sh <name> [node] [--keep]

./remove-vm.sh myvm pve --keep   # keep the directory after destroy
```

### `provision.sh`

Provisions over SSH by default, or on the current host with `--local`. It waits
for SSH and cloud-init, installs package prerequisites and the CLI toolchain,
configures mise shims, `~/.local/bin`, `~/.env`, and direnv in `~/.bashrc`, and
makes `~/.bash_profile` source it. It then installs profile-selected terminal
and fonts and retrieves OpenBao env secrets and kubeconfigs. Local mode verifies
system prerequisites without sudo and installs tools per-user.

Remote mode stages scripts once under `/tmp/homelab-provision/`, preserving
relative paths and vendored installers. It copies only `secrets/get-*`, never
privileged `admin/set-*`, and removes the staging directory on exit.

Each step uses one SSH invocation so piped OpenBao credentials remain intact.
The script forwards tokens when available; otherwise it prompts once and
reuses the password. Provisioning waits up to 600 seconds for cloud-init;
override with `CLOUD_INIT_WAIT_TIMEOUT`.

```bash
./provision.sh <ip> [username]      # remote: push to the VM at <ip> over SSH
./provision.sh --local [username]    # local: provision this machine directly
./provision.sh --profile desktop <ip> [username]

CLOUD_INIT_WAIT_TIMEOUT=900 ./provision.sh 192.168.20.50 myuser
```

`--local` requires Ubuntu, Debian, or Rocky and runs as `$USER` without
SSH or staging; it cannot provision another user.

The machine profile defaults to `auto`: `provision.sh` first reads the
root-owned `/etc/provisioning/machine-profile.local` marker baked by Packer and
otherwise uses the safe `server` default. `--profile desktop|server` or the
`TOOL_MACHINE_PROFILE` environment variable overrides auto mode. Kitty, UDEV
Gothic and Freelens are desktop-only components. Older desktop images without
the marker must be provisioned with `--profile desktop`.

Manage per-user kitty preferences in the dotfiles repository. This script
installs kitty and its font but does not edit `~/.config/kitty/kitty.conf`.

### `check-image-refs.sh`

Checks image filename maps in VM/Packer scripts against Terraform definitions.
CI runs it on relevant changes; run it after adding or renaming a target.

```bash
./check-image-refs.sh
```

### `check-proxy-routes.sh`

Checks Caddy sites and Homepage links in both directions: every UI site needs a
tile, and every HTTPS `home.butaco.net` tile without a port needs a Caddy site.

Explicit-port and plain-HTTP links bypass Caddy and are skipped. `NO_UI_HOSTS`
lists machine-only routes. Only `home.butaco.net` is in scope; cluster domains
use the Gateway. CI runs the check on either source file changing.

```bash
./check-proxy-routes.sh
```

### `check-openbao-secrets.sh`

Reports declared paths missing from OpenBao and stored paths without a
declaration. Remove retired secrets from `openbao_secrets`; deleting them only
from the server lets the next seed run recreate them.

Paths written by `secrets/admin/` are declared in the script's
`UNMANAGED_PATHS`; extend it when a new `set-*.sh` writes to a new path. Argo CD
paths are derived from `openbao_argocd_admin`.

Reports only, and reads path names alone — no secret value is extracted, and
nothing is deleted. Exits `1` when either direction has a finding. It needs the
AGE key and network access to OpenBao, so unlike the other `check-*` scripts it
does not run in CI. `kv-read` is enough; the default user is `homelab`.

```bash
./check-openbao-secrets.sh
BAO_TOKEN=xxx ./check-openbao-secrets.sh
```

## Secrets / environment

Secret scripts run on the host where they are invoked; they do not implement
SSH. `provision.sh` stages and invokes the env and kubeconfig retrieval scripts
over SSH in remote mode.

Authentication order is `BAO_TOKEN`, token stdin when `BAO_TOKEN_STDIN=1`,
`BAO_PASSWORD`, then a TTY prompt or password stdin. Tokens bypass userpass;
unset `BAO_TOKEN` and `BAO_TOKEN_STDIN` to retry with password authentication.

Common env vars: `OPENBAO_ADDR` (default `https://openbao.home.butaco.net`),
`BAO_USERNAME`, `BAO_TOKEN`, `BAO_TOKEN_STDIN`, `BAO_PASSWORD`. Export credentials
once for non-interactive use; the same authentication applies to all `secrets/` scripts.

### `get-env.sh`

Fetches `secret/provision/env` atomically into `~/.env`. Variable references
expand when sourced; the script rejects command substitutions.

```bash
./secrets/get-env.sh
```

### `set-env.sh`

Stores `~/.env` in `secret/provision/env` as admin. Parsing does not source or
execute the file, so variables remain literal.

```bash
./secrets/admin/set-env.sh
```

### `get-kubeconfig.sh`

Retrieves the `prd`/`sandbox` kubeconfigs from OpenBao into `~/.kube/`. Existing
files remain intact until the script successfully fetches both kubeconfigs.

```bash
./secrets/get-kubeconfig.sh                       # local, interactive
```

### `set-kubeconfig.sh`

Stores `~/.kube/prd.yaml` and `~/.kube/sandbox.yaml` in OpenBao at
`secret/kubeconfig/prd` and `secret/kubeconfig/sandbox`. Defaults to the `admin`
OpenBao user and validates both files before writing either secret.

```bash
./secrets/admin/set-kubeconfig.sh
```

### `get-sops-key.sh`

Fetches and validates `secret/sops/age` atomically into `SOPS_AGE_KEY_FILE` or
the default age key path. Normal provisioning does not invoke this script;
run it only on hosts that need repository decryption to limit AGE key distribution.

```bash
./secrets/get-sops-key.sh                       # local, interactive
```

### `set-sops-key.sh`

Stores `~/.config/sops/age/keys.txt` (override with `SOPS_AGE_KEY_FILE`) in
OpenBao at `secret/sops/age`. Defaults to the `admin` OpenBao user and validates
the file before writing.

```bash
./secrets/admin/set-sops-key.sh
```

### `seed-mcp.sh`

`secrets/admin/seed-mcp.sh` reads `.env/secrets.sops.env` and writes changed
upstream and caller tokens to `secret/k8s/mcp/{grafana,netbox}`. It defaults to
the `admin` user and reports paths and status only. Follow
[k8s/mcp/README.md](../k8s/mcp/README.md) for policy setup and coordinated rotation.

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

`starting (0/1)` means desired but not ready, including GPU contention. Replica
count is runtime state outside Git, so status comes from the cluster.

Targets come from the `homelab/gpu-switchable` Deployment label shared with Argo
CD health (ADR-0027). An unknown name lists available targets.

## Grafana MCP

OpenCode uses the Grafana stdio launcher through `opencode.json` to query
PromQL, LogQL, and dashboards at `https://grafana.prd.butaco.net`. Codex and
Claude Code use [cluster-hosted HTTP MCP](../k8s/mcp/README.md) through
`.codex/config.toml` and `.mcp.json`; stdio remains the cluster-outage fallback.

### `grafana-mcp.sh`

The client starts the stdio launcher with `-i` and no TTY. Both MCP launchers
use `lib/mcp-launcher.sh` to select OrbStack's `docker` on macOS or `podman`
otherwise, and load exported credentials or `.env/secrets.sops.env` relative
to the repository. Containers receive environment variable names rather than
credential values on command lines. A running container runtime is required.

The launcher pins the Grafana image tag in [grafana-mcp.sh](grafana-mcp.sh),
and Renovate tracks it. Default tools cover search, datasources, Prometheus,
Loki, dashboards, navigation, and alerting. The launcher disables writes to keep MCP
inspection read-only and caps Loki responses at 20 lines to bound log output
per query.

Use an absolute launcher path in global client configuration so it works
from any working directory. The client invokes the launcher without a shell
and does not expand `~`. Example for Codex (`~/.codex/config.toml`):

```toml
[mcp_servers.grafana]
command = "/absolute/path/to/homelab/scripts/grafana-mcp.sh"
startup_timeout_ms = 60000   # first run pulls the grafana/mcp-grafana image
```

For the repository-local HTTP configuration and token rotation, see
[k8s/mcp/README.md](../k8s/mcp/README.md).

### OpenCode setup

The repo-root `opencode.json` uses the native OpenCode v2 config shape and
registers `grafana` as a local MCP server with a 60-second startup timeout.
OpenCode does not read the `mcpServers` shape in `.mcp.json`; it starts the same
launcher through its own `mcp.servers.grafana` entry. A top-level `grafana_*`
permission denies the tools to normal agents; only the read-only `grafana`
subagent allows them.

Verify the connection from the repository root:

```bash
opencode mcp list
```

In an interactive session, invoke the read-only tool loop explicitly with
`@grafana`, for example:

```text
@grafana List each Grafana data source name and type.
```

### Goose workstation setup

The validated baseline is:

| Setting | Value |
|---------|-------|
| Agent | Goose |
| Provider | Ollama |
| Endpoint | `https://ollama.prd.butaco.net` |
| Model | `qwen3:14b` |
| Context | `32768` (Goose baseline; Ollama deployment defaults to `131072`) |
| Mode | `approve` |
| Maximum turns | `10` |
| Telemetry | disabled |
| Grafana MCP | local stdio launcher, enabled |

Install and configure the client interactively:

```bash
brew install block-goose-cli
goose configure
```

Select the Ollama settings above and add a `grafana` command-line extension:

```text
/absolute/path/to/homelab/scripts/grafana-mcp.sh
```

Use a 300-second extension timeout without token variables. Keep
`GOOSE_MODE=approve`; use one-shot `auto` only for narrow, read-only tasks.

Smoke-test the complete tool loop from Goose:

```text
Use only the Grafana MCP. List each Grafana data source name and type.
Do not modify Grafana or any files.
```

`list_datasources` should return Prometheus and Loki.

If the MCP server does not start, run `bash -n scripts/grafana-mcp.sh`, confirm
that Docker or Podman is running, and check that `sops` can access the AGE key.
If model requests end at a fixed duration, inspect the Ollama HTTPRoute request
and backend-request timeouts before changing Goose's extension timeout.

| Env var | Default | Notes |
|---------|---------|-------|
| `GRAFANA_MCP_RUNTIME` | `docker` on macOS with OrbStack, otherwise `podman` | Force a runtime |
| `GRAFANA_MCP_VERSION` | [grafana-mcp.sh](grafana-mcp.sh) | Pinned image tag; Renovate bumps this in place |
| `GRAFANA_MCP_IMAGE`   | `docker.io/grafana/mcp-grafana:$GRAFANA_MCP_VERSION` | Full reference override for testing |
| `GRAFANA_MCP_ENABLED_TOOLS` | `search,datasource,prometheus,loki,dashboard,navigation,alerting` | Comma-separated tool categories |
| `GRAFANA_MCP_DISABLE_WRITE` | `true` | Set to `false` only for an explicitly approved write workflow |
| `GRAFANA_MCP_MAX_LOKI_LOG_LIMIT` | `20` | Maximum log lines returned by a Loki query |

On macOS, the Podman fallback expects a working Podman machine. Prefer the
official Podman macOS installer; Homebrew builds can miss VM helper binaries
such as `krunkit`. After installing Podman, initialize the machine with
`podman machine init --now`.

### `grafana-mcp-token.sh`

Creates/reuses the Viewer service account and prints a new dotenv token line.
Admin auth comes from environment or the explicitly selected cluster secret.

```bash
# Print the token line
./grafana-mcp-token.sh

# Store it: paste the printed line over the existing assignment
sops edit ../.env/secrets.sops.env
direnv allow
```

Use `sops edit` to replace the assignment; do not append plaintext or re-encrypt
the existing SOPS file.

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
image over stdio with read-only query tools. It is the fallback for
[cluster-hosted MCP](../k8s/mcp/README.md) and uses the shared credential and
runtime setup described under [Grafana MCP](#grafana-mcpsh).

The launcher pins the image tag in [netbox-mcp.sh](netbox-mcp.sh). Its upstream `uv.lock`
pins transitive dependencies that a tagged `uvx` install would re-resolve.

Generate the desired v2 token, then store it by editing the existing encrypted
environment file:

```bash
./netbox-mcp-token.sh
sops edit ../.env/secrets.sops.env
direnv allow
```

Add this assignment inside the decrypted editor buffer:

```bash
NETBOX_TOKEN="<read-only-api-token>"
```

The NetBox role creates the read-only identity and matching v2 token. NetBox
stores only an HMAC, so generate the plaintext locally, then preview and apply:

```bash
cd ../ansible
ansible-playbook playbooks/services/netbox.yaml --check --diff --tags netbox

# The operator performs the live change after reviewing the check output.
ansible-playbook playbooks/services/netbox.yaml --tags netbox
```

Identity steps use `manage.py` and are skipped in check mode. Rotate by
regenerating, editing SOPS, and rerunning the play. See the role README for scope.

Do not commit a plaintext token or pass it in `.codex/config.toml` or
`.mcp.json`. Start a new client session after changing MCP configuration. The
first launch pulls the image, so it needs network access; later launches reuse
the local image.

| Env var | Default | Notes |
|---------|---------|-------|
| `NETBOX_URL` | `https://netbox.home.butaco.net/` | Target NetBox instance |
| `NETBOX_TOKEN` | none | Required read-only API token |
| `VERIFY_SSL` | `true` | Keep TLS verification enabled |
| `ENABLE_PLUGIN_DISCOVERY` | `false` | Enable only when NetBox plugin models are needed |
| `NETBOX_MCP_VERSION` | [netbox-mcp.sh](netbox-mcp.sh) | Pinned image tag; Renovate bumps this in place |
| `NETBOX_MCP_IMAGE` | `docker.io/netboxlabs/netbox-mcp-server:$NETBOX_MCP_VERSION` | Full reference override for testing |
| `NETBOX_MCP_RUNTIME` | auto (OrbStack `docker`, else `podman`) | Force a container runtime |

Renovate tracks both launcher tags through script comments. Updates create
manual-review PRs because scripts are outside the automerge paths.

## `install/`

### `packages.sh`

Runs the vendored privileged package installer before tools and fonts.

Set `TOOL_SKIP_SYSTEM_PACKAGES=1` to perform a no-sudo preflight instead of
installing packages. This is used by `provision.sh --local`, where a golden image
is expected to provide the prerequisites already.

`TOOL_MACHINE_PROFILE=desktop|server` controls desktop-only system packages,
including Freelens and the fontconfig dependency used by `fonts.sh`.

```bash
./install/packages.sh                              # install packages via sudo
TOOL_SKIP_SYSTEM_PACKAGES=1 ./install/packages.sh  # no-sudo preflight
./install/packages.sh global                       # system-wide version cache
```

### Tools, terminal, and fonts

`tools.sh` runs mise with the vendored config and lockfile managed in dotfiles.
Local mode installs mise and tools per user; global mode installs the
golden-image baseline system-wide. Both use `mise install --locked` so a build
does not resolve releases through GitHub APIs. User shims take precedence over
system shims, and project-specific mise configs can override the baseline.
`terminal.sh` installs kitty and `fonts.sh` installs UDEV Gothic NF; both skip
installation unless `TOOL_MACHINE_PROFILE=desktop`.

Tools, terminal, and fonts accept `local` (default, no sudo) or `global`
(system-wide, needs sudo). For packages, the mode selects the version-cache path;
package installation requires sudo unless the no-sudo preflight is selected:

| Script | Local path | Global path |
|--------|------------|-------------|
| `packages.sh` (version cache) | `$HOME/.local/share/tool-versions` | `/usr/local/share/tool-versions` |
| `tools.sh` | `$HOME/.local/share/mise` | `/usr/local/share/mise` |
| `terminal.sh` | `$HOME/.local/kitty.app` | `/usr/local/kitty.app` |
| `fonts.sh` | `$HOME/.local/share/fonts` | `/usr/local/share/fonts` |

```bash
./install/tools.sh
./install/tools.sh global
```

### `vendor/`

Renovate advances the recorded dotfiles `main` SHA; GitHub Actions synchronizes
files into the same PR. `sync.sh` uses the recorded SHA, `--check` verifies
without writing, and `--upstream` advances the pin explicitly.

Vendored installers, mise config, and lockfile remove the runtime dependency on
the dotfiles repository.

Do not edit the numbered installer scripts by hand — they keep the chezmoi
target names of their sources and are synced from `takanao14/dotfiles` by
`vendor/sync.sh`:

```bash
./install/vendor/sync.sh              # re-sync from the SHA in vendor/REVISION
./install/vendor/sync.sh --upstream   # advance the pin to the latest dotfiles main
REF=<sha|tag> ./install/vendor/sync.sh   # pin to a specific ref
./install/vendor/sync.sh --check      # CI: compare with vendor/REVISION
```
