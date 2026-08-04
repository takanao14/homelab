# code_server Role

Installs [code-server](https://github.com/coder/code-server) (browser-based VS
Code) on Debian-based systems and runs it as a systemd service for one account.

Access is over HTTPS through Caddy (`vscode.home.butaco.net`); code-server
itself serves plain HTTP and is never published to the internet.

## Functionality

- Downloads the release `.deb` from GitHub and installs it with `apt`.
- Records the installed version under `/usr/local/share/homelab-versions`
  for `playbooks/ops-version_audit.yaml`.
- Deploys `~/.config/code-server/config.yaml` (bind address, argon2 password).
- Seeds `settings.json` once, then leaves editor preferences to the user.
- Deploys a systemd drop-in pinning the workspace folder.
- Enables and starts `code-server@<user>`.

## Why it runs as a real login account

The integrated terminal is the point of this service: the coding agents
(`claude`, `codex`) run in it. Running under the account that already owns
`~/.claude`, `~/.codex`, the SSH keys, the git config and the SOPS AGE key
avoids duplicating all of that. The role therefore expects the account to
exist and never creates it.

The consequence is that anyone who can log in to the web UI has that account's
shell, including its `sudo` rights. The password is the only barrier, so keep
the service off the internet and use a long random password.

## Variables

### Secrets (set in `group_vars/code_server.sops.yaml`)

| Variable | Description |
|----------|-------------|
| `code_server_hashed_password` | argon2 hash of the login password |

Generate the hash and store the output. Read the password interactively rather
than passing it as an argument: it stays out of shell history, and the shell
cannot expand `$`, `` ` `` or `!` inside it.

```bash
read -rs "pw?password: " && printf '%s' "$pw" | argon2 "$(openssl rand -hex 8)" -e; unset pw
```

`printf '%s'` matters. The argon2 CLI hashes stdin verbatim, so `echo` without
`-n` hashes the password *plus a newline* — the result is a valid-looking hash
that no browser login can ever match. `npx argon2-cli -e` works equally well
and takes no salt argument; the same newline caveat applies.

To check an existing hash against a password, re-derive it with the salt from
the stored value (field 5 of the `$`-separated encoded string, base64-decoded)
and compare the strings.

### Non-secret variables (in `defaults/main.yaml`)

| Variable | Default | Description |
|----------|---------|-------------|
| `code_server_version` | `4.131.0` | Release to install (Renovate-tracked) |
| `code_server_user` | (required) | Account the editor runs as |
| `code_server_group` | `{{ code_server_user }}` | Primary group of that account |
| `code_server_home` | `/home/{{ code_server_user }}` | Home directory |
| `code_server_workspace` | `{{ code_server_home }}/lab` | Folder opened on connect |
| `code_server_binary` | `/usr/bin/code-server` | Binary path (set by the package) |
| `code_server_bind_addr` | `0.0.0.0:8080` | Listen address |
| `code_server_hostname` | `vscode.home.butaco.net` | Public name served by Caddy |

`code_server_bind_addr` is not loopback because Caddy runs on `caddy1`, on a
different subnet.

## Extensions

code-server uses Open VSX, not the Microsoft Marketplace. The official
Anthropic `Anthropic.claude-code` extension is published there. Whether a given
extension works in a browser-hosted editor still has to be confirmed per
extension.

The extension bundles its own CLI copy for the chat panel; running `claude` in
the integrated terminal needs the standalone CLI installed separately. That
install is not managed by this role.

## Dependencies

None.

## Usage

```yaml
- name: Setup code-server
  hosts: code_server
  roles:
    - role: code_server
```
