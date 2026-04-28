# PowerDNS Authoritative Role (pdns_auth)

Installs and configures PowerDNS Authoritative Server with an SQLite3 backend on Debian-based systems.

Supports both `primary` (master) and `secondary` (slave) configurations.

## Functionality

- Adds the official PowerDNS APT repository and configures APT pinning for `pdns-auth`.
- Installs `pdns-server`, `pdns-backend-sqlite3`, and `sqlite3`.
- Validates required variables before proceeding.
- Initializes the SQLite database if it doesn't exist.
- Deploys `/etc/powerdns/pdns.conf` based on the role (`primary` or `secondary`).
- Enables and starts the `pdns` service.
- Registers the primary as an autoprimary on the secondary node via the PowerDNS API.

## Variables

### Role-defining variable

```yaml
# Set in group_vars (e.g., inventories/homelab/group_vars/dns_primary.yaml)
pdns_role: primary   # or secondary
```

### Secrets (from SOPS-encrypted group_vars)

Loaded via `community.sops.sops` from `group_vars/dns_primary.sops.yaml` and `group_vars/dns_secondary.sops.yaml`.

| Variable | Description |
|----------|-------------|
| `PDNS_PRIMARY_API_KEY` | API key for the primary PowerDNS server |
| `PDNS_SECONDARY_API_KEY` | API key for the secondary PowerDNS server |

These are mapped to `pdns_primary_api_key` / `pdns_secondary_api_key` in `defaults/main.yaml`.

### Shared variables (from `group_vars/dns_auth.yaml`)

| Variable | Description |
|----------|-------------|
| `primary_auth_server` | `host:port` of the primary auth server |
| `secondary_auth_server` | `host:port` of the secondary auth server |
| `pdns_webserver_allow_from` | Allowed IP ranges for API access (e.g. `192.168.10.0/24,...`) |

`primary_auth_server` and `secondary_auth_server` are split into `pdns_primary_addr`/`pdns_primary_port` and `pdns_secondary_addr`/`pdns_secondary_port` in `group_vars/dns_auth.yaml`.

### Primary-specific variables (from `group_vars/dns_primary.yaml`)

| Variable | Description |
|----------|-------------|
| `pdns_primary_allow_axfr_ips` | IPs allowed to perform zone transfers |
| `pdns_primary_also_notify` | IPs to NOTIFY on zone change (defaults to `{{ secondary_auth_server }}`) |
| `pdns_primary_nameserver` | FQDN of the primary nameserver, used for autoprimary registration |
| `pdns_default_soa_content` | Default SOA record content for new zones |

### Defaults (in `defaults/main.yaml`)

Overridable repository and webserver settings.

| Variable | Default | Description |
|----------|---------|-------------|
| `pdns_webserver_port` | `8081` | API/webserver listen port |
| `pdns_repo_channel` | `auth-50` | PowerDNS APT repository channel |
| `pdns_repo_release` | `{{ ansible_facts['distribution_release'] }}` | Ubuntu release codename |

## Dependencies

None.

## Usage

```yaml
# In playbooks/dns.yml
- name: Setup Primary DNS Server
  hosts: dns_primary
  roles:
    - role: pdns_auth

- name: Setup Secondary DNS Server
  hosts: dns_secondary
  roles:
    - role: pdns_auth
```
