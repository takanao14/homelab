# Hermes Agent evaluation

`hermes1` runs Ubuntu 26.04 with the pinned Hermes Agent container image. It
uses the same Lemonade endpoint and model as `openclaw1`. Only SSH users who
can run Podman through sudo can start the CLI; no Hermes port is published.

Deploy with `ansible-playbook playbooks/services/hermes.yaml` from `ansible/`.
Then run `ssh -t takanao@192.168.20.24 hermes-eval` for an interactive conversation.
For a baseline with only clarification tools, run
`hermes-eval chat --toolsets clarify`. Keep the same prompt and model when
comparing responses with OpenClaw, and run the evaluations sequentially because
both share one GPU.

The configuration enables memory and skills but requires approval before either
is written. Sessions and agent state persist under `/var/lib/hermes`; back up
that directory before replacing the VM. The fixed container name prevents two
evaluation sessions from writing the same state concurrently.
Lemonade must be selected in gpu-switch before inference requests can succeed.

Grafana and NetBox MCP tools use the same allowlist as OpenClaw in
`group_vars/mcp_client.yaml`. The shared `mcp_client` role reads caller tokens
from `.env/secrets.sops.env` and writes `/etc/hermes/mcp.env` as root with mode
`0600`. Podman passes them as environment variables on the next CLI launch;
the YAML config contains references only. VM backups contain this plaintext
file. Rotate server and client tokens together using `k8s/mcp/README.md`.

The `web` toolset uses Parallel's anonymous free tier for search and page
extraction; queries and requested URLs are sent to Parallel. No extra API key
is required. Free-tier requests may be rate-limited. Private-address protection
remains enabled, and browser automation and terminal tools remain disabled.
