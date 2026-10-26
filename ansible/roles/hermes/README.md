# Hermes Agent evaluation

`hermes1` runs Ubuntu 26.04 with the pinned Hermes Agent image in rootless Podman
under `hermes-svc`. It uses the same Lemonade endpoint and model as `openclaw1`.
The Web Dashboard is
served at `https://hermes.home.butaco.net` through Caddy
over VPN. The VM firewall admits port `9119` only from Caddy.

Deploy with `ansible-playbook playbooks/services/hermes.yaml` from `ansible/`.
Log in as `takanao` with `hermes_dashboard_password` from the SOPS-encrypted
`group_vars/hermes.sops.yaml` file. The VM receives only its scrypt hash and a
session-signing key in `0600` `/etc/hermes-rootless/dashboard.env`, owned by
`hermes-svc` under a root-owned parent directory. Keep the
password, hash, and signing key consistent when rotating credentials.
SSH CLI access remains available with
`ssh -t takanao@192.168.20.24 hermes-eval`.
For a baseline with only clarification tools, run
`hermes-eval chat --toolsets clarify`. Keep the same prompt and model when
comparing responses with OpenClaw, and run the evaluations sequentially because
both share one GPU.

The configuration enables memory and skills but requires approval before either
is written. Sessions and agent state persist under `/var/lib/hermes-rootless`.
Recovery rebuilds empty state from Ansible/SOPS; backups are not required,
and sessions and learned state are disposable. The Dashboard and SSH CLI share the
systemd-managed `hermes-dashboard`
container and persistent state. Avoid concurrent model requests during comparisons.
Lemonade must be selected in gpu-switch before inference requests can succeed.

Grafana and NetBox MCP tools use the same allowlist as OpenClaw in
`group_vars/mcp_client.yaml`. The shared `mcp_client` role reads caller tokens
from `.env/secrets.sops.env` and writes `/etc/hermes-rootless/mcp.env` as `hermes-svc` with mode
`0600`. Podman passes them as environment variables when the dashboard restarts;
the YAML config contains references only. VM backups contain this plaintext
file. Rotate server and client tokens together using `k8s/mcp/README.md`.

The `web` toolset uses Parallel's anonymous free tier for search and page
extraction; queries and requested URLs are sent to Parallel. No extra API key
is required. Free-tier requests may be rate-limited. Private-address protection
remains enabled, and browser automation and terminal tools remain disabled.

The homelab inventory enables rootless operation with fresh state.
Other inventories must explicitly opt in using the
[shared procedure](../podman/README.md#opt-in-rootless-services). The system service
name, root firewall ordering, and restart notifications stay unchanged.
