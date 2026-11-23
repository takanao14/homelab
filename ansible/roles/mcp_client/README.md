# mcp_client Role

Deploys read-only MCP caller bearer tokens from `.env/secrets.sops.env` to a
mode-`0600` environment file on agent hosts (`openclaw`, `hermes`).

## Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `mcp_client_env_file` | (required) | Destination environment file path on the target host |
| `mcp_client_secret_file` | `{{ inventory_dir }}/../../../.env/secrets.sops.env` | Controller-side SOPS dotenv file holding `MCP_GRAFANA_SERVER_TOKEN` and `MCP_NETBOX_SERVER_TOKEN` |
| `mcp_client_owner` | `root` | Owner of the rendered environment file |
| `mcp_client_group` | `{{ mcp_client_owner }}` | Group of the rendered environment file and its parent directory (`0750`) |
| `mcp_client_notify` | `[]` | Handlers notified when the environment file changes |

## Usage

Included by [playbooks/services/openclaw.yaml](../../playbooks/services/openclaw.yaml)
and [playbooks/services/hermes.yaml](../../playbooks/services/hermes.yaml).
