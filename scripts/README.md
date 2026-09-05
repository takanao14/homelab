# scripts

Homelab VM lifecycle, provisioning, secret sync, checks, and integrations.

## Layout

```
scripts/
├── create-vm.sh / remove-vm.sh / provision.sh  # VM lifecycle (run directly)
├── gpu-switch.sh                             # k8s GPU workload switch
├── check-image-refs.sh                       # CI: image filename map consistency check
├── check-proxy-routes.sh                     # CI: Caddy sites <-> Homepage dashboard drift check
├── check-openbao-secrets.sh                  # declared secret paths <-> OpenBao KV drift check (local only)
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
| image  | `ubuntu24-base` | `ubuntu24-{base,tool,desktop}`, `rocky10-base`, `rocky9-{base,desktop}`, `debian13-base` |

The role suffix says how much is baked in: `base` (guest agent + timezone) <
`tool` (+ shared CLI toolchain) < `desktop` (+ XFCE/XRDP and the GUI
applications). See [packer/README.md](../packer/README.md).

The `tool` and `desktop` images are downloaded only on `pve`, so VMs using them
must be created there.

VM credentials use `TF_VM_USERNAME`, `TF_VM_PASSWORD`, and
`TF_VM_SSH_PUBLIC_KEY`. Defaults are the current user, a hidden password prompt,
and `~/.ssh/id_ed25519.pub`; non-interactive runs must set the password. Resolved
values are reused from a mode-600, gitignored `.envrc` containing plaintext
credentials. Keep it local and run `direnv allow` once.

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
6. Fetches env secrets from OpenBao into `~/.env` (`secrets/get-env.sh`)
7. Retrieves kubeconfigs from OpenBao into `~/.kube/` (`secrets/get-kubeconfig.sh`)

Remote mode stages scripts once under `/tmp/homelab-provision/`, preserving
relative paths and vendored installers. It copies only `secrets/get-*`, never
privileged `admin/set-*`, and removes the staging directory on exit.

Each step uses one SSH invocation so piped OpenBao credentials remain intact.
Tokens are forwarded; otherwise one prompted password is reused. Provisioning
waits up to 600 seconds for cloud-init; override with `CLOUD_INIT_WAIT_TIMEOUT`.

```bash
./provision.sh <ip> [username]      # remote: push to the VM at <ip> over SSH
./provision.sh --local [username]    # local: provision this machine directly
./provision.sh --profile desktop <ip> [username]

./provision.sh 192.168.20.50 myuser
CLOUD_INIT_WAIT_TIMEOUT=900 ./provision.sh 192.168.20.50 myuser
./provision.sh --local               # run on the target Linux box as that user
```

`--local` runs directly on the target Ubuntu, Debian, or Rocky host as `$USER`.
It skips SSH/staging, performs a no-sudo package preflight, and installs remaining
tools per-user under `$HOME/.local`.

The machine profile defaults to `auto`: `provision.sh` first reads the
root-owned `/etc/provisioning/machine-profile.local` marker baked by Packer and
otherwise uses the safe `server` default. `--profile desktop|server` or the
`TOOL_MACHINE_PROFILE` environment variable overrides auto mode. Kitty, UDEV
Gothic and Freelens are desktop-only components. Older desktop images without
the marker must be provisioned with `--profile desktop`.

Per-user kitty preferences are managed by the dotfiles repository. This script
installs kitty and its font but does not edit `~/.config/kitty/kitty.conf`.

### `check-image-refs.sh`

Checks image filename maps in VM/Packer scripts against Terraform definitions.
CI runs it on relevant changes; run it after adding or renaming a target.

```bash
./check-image-refs.sh
```

### `check-proxy-routes.sh`

Checks Caddy sites against Homepage tiles in both directions:

- a `caddy_upstreams` / `caddy_redirects` hostname with no dashboard entry —
  the service is published but invisible on the portal
- a dashboard `https://<host>.home.butaco.net` link (no port, plain `https`)
  that Caddy does not serve — a dead tile

Explicit-port and plain-HTTP links bypass Caddy and are skipped. `NO_UI_HOSTS`
lists machine-only routes. Only `home.butaco.net` is in scope; cluster domains
use the Gateway. CI runs the check on either source file changing.

```bash
./check-proxy-routes.sh
```

### `check-openbao-secrets.sh`

Checks declared secret paths against the OpenBao KV store in both directions:

- a path declared in SOPS but absent from the server — the next
  `ops-openbao_seed_secrets.yaml` run recreates it, so a retired secret must be
  removed from `openbao_secrets`, not only from the server
- a path on the server that nothing declares — an orphan left by an out-of-band
  write

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

OpenBao scripts run locally or over SSH. Authentication order is `BAO_TOKEN`,
`BAO_PASSWORD`, TTY prompt, then non-interactive stdin. A token bypasses userpass;
unset it to retry with password authentication.

Common env vars: `OPENBAO_ADDR` (default `https://openbao.home.butaco.net`),
`BAO_USERNAME`, `BAO_TOKEN`, `BAO_PASSWORD`.

### `get-env.sh`

Fetches `secret/provision/env` atomically into `~/.env`. Variable references
expand when sourced; command substitutions are rejected.

```bash
./secrets/get-env.sh
BAO_TOKEN=xxx ./secrets/get-env.sh
```

### `set-env.sh`

Stores `~/.env` in `secret/provision/env` as admin. Parsing does not source or
execute the file, so variables remain literal.

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

Fetches and validates `secret/sops/age` atomically into `SOPS_AGE_KEY_FILE` or
the default age key path. It is intentionally excluded from normal provisioning;
run it only on hosts that need repository decryption.

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

`starting (0/1)` means desired but not ready, including GPU contention. Replica
count is runtime state outside Git, so status comes from the cluster.

Targets come from the `homelab/gpu-switchable` Deployment label shared with Argo
CD health (ADR-0027). An unknown name lists available targets.

## Grafana MCP

Scripts backing the Grafana MCP server registered in the repo-root `.mcp.json`
and `opencode.json`. The server lets an MCP client (Claude Code, Codex,
OpenCode, Cursor, …) query Grafana (PromQL/LogQL and dashboards) against
`https://grafana.prd.butaco.net`.

### `grafana-mcp.sh`

Launcher invoked by the MCP client over stdio. It selects a container runtime
per OS — `docker` on macOS when OrbStack is installed, `podman` otherwise — and
runs the `grafana/mcp-grafana` image with `-i` (no TTY). You normally don't run
this by hand; the client starts it.

The vendor image is release-pinned and Renovate-managed; Docker's curated mirror
offers only `latest`. The binary reports `(devel)`, so trust the image tag.

Default tools cover read-only search, data sources, Prometheus, Loki, dashboards,
navigation, and alerting. Alerting tools expose rule state plus Alertmanager
notification policies, contact points, and time intervals. Writes are disabled
and Loki responses capped at 20 lines.

The launcher uses exported credentials or decrypts `.env/secrets.sops.env`
relative to itself. It works from any cwd without embedding tokens in clients.

A global client config is read from any working directory, so it needs the
absolute path; the launcher is not resolved through a shell, so `~` is not
expanded. Example for Codex (`~/.codex/config.toml`):

```toml
[mcp_servers.grafana]
command = "/absolute/path/to/homelab/scripts/grafana-mcp.sh"
startup_timeout_ms = 60000   # first run pulls the grafana/mcp-grafana image
```

The repository-local `.codex/config.toml` instead pairs a relative `command`
with `cwd = "."`, mirroring `.mcp.json`, so no absolute path is baked in.

### OpenCode setup

The repo-root `opencode.json` registers `grafana` as a local MCP server with a
60-second startup timeout. OpenCode does not read the `mcpServers` shape in
`.mcp.json`; it starts the same launcher through its own `mcp.grafana` entry.
Grafana tools are disabled for normal agents and enabled only in the read-only
`grafana` subagent, keeping their schemas out of ordinary model requests.

Verify the connection from the repository root:

```bash
opencode mcp list --pure
```

In an interactive session, invoke the read-only tool loop explicitly with
`@grafana`, for example:

```text
@grafana List each Grafana data source name and type.
```

The 2026-09-04 validation connected successfully and returned the Loki and
Prometheus data sources through `grafana_list_datasources`.

### Goose workstation setup

Run Goose on the workstation and keep Ollama in-cluster for repository access
and interactive approval without another in-cluster agent.

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

Success means `list_datasources` returns Prometheus and Loki. Broader checks may
search a dashboard and inspect its summary and queries.

If the MCP server does not start, run `bash -n scripts/grafana-mcp.sh`, confirm
that Docker or Podman is running, and check that `sops` can access the AGE key.
If model requests end at a fixed duration, inspect the Ollama HTTPRoute request
and backend-request timeouts before changing Goose's extension timeout.

| Env var | Default | Notes |
|---------|---------|-------|
| `GRAFANA_MCP_RUNTIME` | `docker` on macOS with OrbStack, otherwise `podman` | Force a runtime |
| `GRAFANA_MCP_VERSION` | `1.3.0` | Pinned image tag; Renovate bumps this in place |
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

The file is already encrypted; append-then-encrypt fails on existing SOPS
metadata. Rotate through `sops edit`.

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

The image includes upstream `uv.lock`, pinning transitive dependencies; a tagged
`uvx` install would re-resolve them. Upstream signs its multi-arch images.

Shared launcher logic uses exported NetBox credentials or SOPS and passes only
environment variable names to the container, keeping values off command lines.

Despite upstream's detached-container warning, `docker run -i` without a TTY
preserves stdio framing.

A container runtime must already be running; a cold runtime prevents startup.

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

The NetBox role creates the read-only identity and matching v2 token. NetBox
stores only an HMAC, so generate the plaintext locally, then preview and apply:

```bash
cd ansible
ansible-playbook playbooks/netbox.yaml --check --diff --tags netbox

# The operator performs the live change after reviewing the check output.
ansible-playbook playbooks/netbox.yaml --tags netbox
```

Identity steps use `manage.py` and are skipped in check mode. Rotate by
regenerating, editing SOPS, and rerunning the play. See the role README for scope.

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

### `tools.sh`

Runs the vendored Ubuntu/Rocky CLI installer. Versions are pinned and
Renovate-managed in dotfiles.

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

The installer runs only for `TOOL_MACHINE_PROFILE=desktop`; `server` exits
successfully without installing anything.

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

The installer runs only for `TOOL_MACHINE_PROFILE=desktop`; `server` exits
successfully without installing anything.

| Mode | Target | Sudo |
|------|--------|------|
| `local` (default) | `$HOME/.local/share/fonts` (per-user) | no |
| `global` | `/usr/local/share/fonts` (system-wide, for shared / golden-image VMs) | yes |

```bash
./install/fonts.sh            # local (per-user)
./install/fonts.sh global     # system-wide
```

### `vendor/`

Vendored dotfiles installers remove runtime GitHub dependencies. Their source
commit is recorded in `vendor/REVISION`.

Do not edit the `run_onchange_*.sh` files by hand — they are kept in sync with
`takanao14/dotfiles` by `vendor/sync.sh`:

```bash
./install/vendor/sync.sh           # refresh to the latest dotfiles main
REF=<sha|tag> ./install/vendor/sync.sh   # pin to a specific ref
./install/vendor/sync.sh --check   # CI: fail if the vendored copies have drifted
```
