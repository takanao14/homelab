# dnsdist Role

Installs and configures dnsdist from the official PowerDNS repository on Debian-based systems.

Sets up dnsdist as a DNS load balancer and forwarder: internal domain queries are routed to the PowerDNS Authoritative secondary backends, and external queries go to public resolvers.

## Architecture

dnsdist sits in front of the authoritative DNS servers:

- **Internal domains** (`home.butaco.net.`, `prd.butaco.net.`, `sandbox.butaco.net.`) → routed to the `internal` pool (PowerDNS secondaries only — the hidden primary is excluded)
- **External domains** → routed to the default pool (public resolvers)

The `internal` pool is built dynamically from `secondary_auth_servers`, so adding or removing a secondary only requires updating that list.

## Functionality

- Adds the official PowerDNS APT repository for `dnsdist`.
- Installs the `dnsdist` package.
- Deploys `/etc/dnsdist/dnsdist.conf` from a Jinja2 template.
- Enables and starts the `dnsdist` service.

## Variables

### Secrets (from SOPS-encrypted `group_vars/dnsdist.sops.yaml`)

Loaded via `community.sops.sops` lookup.

| Variable | Description |
|----------|-------------|
| `DNSDIST_WEB_PASSWORD` | Web UI password |
| `DNSDIST_WEB_API_KEY` | Web UI API key |
| `DNSDIST_CONSOLE_KEY` | Console key (generate with `dnsdist --gen-key`) |

These are mapped to `dnsdist_web_password`, `dnsdist_web_api_key`, and `dnsdist_console_key` in `defaults/main.yaml`.

### Backend server lists (from `group_vars/dns.yaml` and `group_vars/dnsdist.yaml`)

| Variable | Description |
|----------|-------------|
| `secondary_auth_servers` | List of `{name, address}` dicts for all secondary auth servers (single source of truth for the `internal` pool) |
| `dnsdist_default_servers` | List of `{name, address, checkName}` dicts for public resolvers (defined in `group_vars/dnsdist.yaml`) |

`dnsdist_internal_backends` and `dnsdist_default_backends` are derived automatically in `defaults/main.yaml` by merging the above lists with pool/checkType fields.

### Role defaults (in `defaults/main.yaml`)

| Variable | Default | Description |
|----------|---------|-------------|
| `dnsdist_server_addr` | Host primary IPv4 | IP address dnsdist binds to (frontend + web UI) |
| `dnsdist_mgmt_acl` | `127.0.0.1/32,192.168.0.0/16,10.0.0.0/8` | ACL for web UI access |
| `dnsdist_console_acl` | `127.0.0.1/32` | ACL for console access |
| `dnsdist_internal_domains` | `home.butaco.net.`, `prd.butaco.net.`, `sandbox.butaco.net.` | Domains routed to the `internal` pool |
| `dnsdist_internal_check_name` | `ns1.home.butaco.net.` | Health check FQDN for internal backends |
| `dnsdist_packet_cache_size` | `10000` | Packet cache entry limit (default pool only) |
| `dnsdist_packet_cache_max_ttl` | `86400` | Maximum TTL for cached entries (seconds) |
| `dnsdist_repo_channel` | `dnsdist-20` | PowerDNS APT repository channel |
| `dnsdist_repo_release` | `{{ ansible_facts['distribution_release'] }}` | Ubuntu release codename |
| `dnsdist_repo_validate_certs` | `true` | Validate TLS cert when fetching the repo key (set to `false` temporarily if repo.powerdns.com cert is expired) |

## Dependencies

None.

## Usage

```yaml
# playbooks/dnsdist.yaml
- name: Setup dnsdist
  hosts: dnsdist
  roles:
    - role: dnsdist
```
