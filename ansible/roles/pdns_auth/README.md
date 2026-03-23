# PowerDNS Authoritative Role (pdns_auth)

Installs and configures PowerDNS Authoritative Server with an SQLite3 backend on Debian-based systems.

Supports both `primary` (master) and `secondary` (slave) configurations.

## Functionality

- Adds the official PowerDNS APT repository for `pdns-auth`.
- Installs `pdns-server`, `pdns-backend-sqlite3`, and `sqlite3`.
- Initializes the SQLite database if it doesn't exist.
- Deploys `/etc/powerdns/pdns.conf` based on the role (`primary` or `secondary`).
- Enables and starts the `pdns` service.

## Variables

### Role-defining variable

```yaml
# Set in group_vars (e.g., inventories/homelab/group_vars/dns_primary.yml)
pdns_role: primary   # or secondary
```

### Secrets (from SOPS-encrypted `secrets.enc.env`)

Loaded via `lookup('ansible.builtin.env', ...)`. Set these in `ansible/secrets.enc.env`.

| Variable | Description |
|----------|-------------|
| `PDNS_PRIMARY_API_KEY` | API key for the primary PowerDNS server |
| `PDNS_SECONDARY_API_KEY` | API key for the secondary PowerDNS server |

### Non-secret variables (in `defaults/main.yml`)

Hardcoded defaults; override per-group or per-host as needed.

| Variable | Default | Description |
|----------|---------|-------------|
| `pdns_webserver_port` | `8081` | API/webserver listen port |
| `pdns_webserver_allow_from` | `192.168.10.0/24,...` | Allowed IP ranges for API access |
| `pdns_primary_allow_axfr_ips` | `192.168.10.0/24` | IPs allowed to perform zone transfers |
| `pdns_primary_also_notify` | `192.168.10.241` | IPs to NOTIFY on zone change |

### Shared variables (from `group_vars/all.yml`)

| Variable | Description |
|----------|-------------|
| `primary_auth_server` | `host:port` of the primary auth server |
| `secondary_auth_server` | `host:port` of the secondary auth server |

These are split into `pdns_primary_addr`/`pdns_primary_port` and `pdns_secondary_addr`/`pdns_secondary_port` in `defaults/main.yml`.

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
