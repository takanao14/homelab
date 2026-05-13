# PowerDNS Authoritative Role (pdns_auth)

Installs and configures PowerDNS Authoritative Server with an SQLite3 backend on Debian-based systems.

Supports `primary` (hidden primary) and `secondary` configurations. Multiple secondary servers are supported.

## Architecture

This role is designed for a hidden primary setup:

- **Primary (ns1)**: Hidden from public NS records. Holds the authoritative zone data and notifies secondaries on change. Not included in dnsdist backend pool.
- **Secondary (ns2, ns3, ...)**: Listed in NS records and dnsdist backend pool. Receive zone transfers from the primary and answer queries.

## Functionality

- Adds the official PowerDNS APT repository and configures APT pinning for `pdns-auth`.
- Installs `pdns-server`, `pdns-backend-sqlite3`, and `sqlite3`.
- Validates required variables before proceeding.
- Initializes the SQLite database if it doesn't exist.
- Deploys `/etc/powerdns/pdns.conf` based on the role (`primary` or `secondary`).
- Enables and starts the `pdns` service.
- Registers the primary as an autoprimary on each secondary node via the PowerDNS API.

## Variables

### Role-defining variable

```yaml
# Set in group_vars (e.g., inventories/homelab/group_vars/dns_primary.yaml)
pdns_role: primary   # or secondary
```

### Secrets (from SOPS-encrypted files)

Primary secret is loaded from `group_vars/dns_primary.sops.yaml`.
Secondary secrets are per-host, loaded from `host_vars/<hostname>.sops.yaml`.

| Variable | Where | Description |
|----------|-------|-------------|
| `PDNS_PRIMARY_API_KEY` | `group_vars/dns_primary.sops.yaml` | API key for the primary PowerDNS server |
| `PDNS_SECONDARY_API_KEY` | `host_vars/<hostname>.sops.yaml` | API key for each secondary (unique per host) |

These are mapped to `pdns_primary_api_key` / `pdns_secondary_api_key` in `defaults/main.yaml`.

To generate a key, use alphanumeric characters only (avoid `=`, `+`, `/` which can cause issues in config files):

```bash
tr -dc 'a-zA-Z0-9' < /dev/urandom | head -c 32 && echo
```

### Shared variables (from `group_vars/dns_auth.yaml`)

| Variable | Description |
|----------|-------------|
| `pdns_primary_addr` | IP address of the primary, derived from `primary_auth_server` |
| `pdns_primary_port` | Listen port of the primary, derived from `primary_auth_server` |
| `pdns_primary_nameserver` | An NS record name that exists in the zone (used for autoprimary registration). Must match a zone NS record — NOT the hidden primary's own name. |
| `pdns_webserver_port` | API/webserver listen port (default: `8081`) |
| `pdns_webserver_allow_from` | Allowed IP ranges for API access (e.g. `192.168.10.0/24,...`) |
| `pdns_repo_validate_certs` | Whether to validate TLS certificates when downloading the repo key (default: `true`) |

### Secondary-specific variables (from `group_vars/dns_secondary.yaml`)

| Variable | Description |
|----------|-------------|
| `pdns_secondary_addr` | IP address of this secondary, set to `{{ ansible_host }}` |
| `pdns_secondary_port` | Listen port, derived from the matching entry in `secondary_auth_servers` |

### Primary-specific variables (from `group_vars/dns_primary.yaml`)

| Variable | Description |
|----------|-------------|
| `pdns_primary_allow_axfr_ips` | IPs/subnets allowed to perform zone transfers |
| `pdns_primary_also_notify` | Addresses to NOTIFY on zone change, built dynamically from `secondary_auth_servers` |
| `pdns_default_soa_content` | Default SOA record content for new zones |

### Server list (from `group_vars/dns.yaml`)

| Variable | Description |
|----------|-------------|
| `primary_auth_server` | `host:port` of the primary auth server |
| `secondary_auth_servers` | List of `{name, address}` dicts for all secondary auth servers |

### Defaults (in `defaults/main.yaml`)

| Variable | Default | Description |
|----------|---------|-------------|
| `pdns_webserver_port` | `8081` | API/webserver listen port |
| `pdns_repo_validate_certs` | `true` | Validate TLS cert when fetching repo key |
| `pdns_repo_channel` | `auth-50` | PowerDNS APT repository channel |
| `pdns_repo_release` | `{{ ansible_facts['distribution_release'] }}` | Ubuntu release codename |

## DNS Record Notes

With a hidden primary setup, the SOA and NS records are configured as follows:

| Record | Value | Reason |
|--------|-------|--------|
| SOA MNAME | `ns1.home.butaco.net.` | ns1 is the true origin of zone data (RFC 1035) |
| NS records | `ns2`, `ns3` only | Only query-answering servers are listed |
| `pdns_primary_nameserver` | `ns2.home.butaco.net.` | Must match a zone NS record for autoprimary verification |

## Dependencies

None.

## Usage

```yaml
# playbooks/pdns_auth.yaml
# Primary must be set up before secondaries so zone transfers can proceed.
- name: Setup Primary DNS Server
  hosts: dns_primary
  roles:
    - role: pdns_auth

- name: Setup Secondary DNS Servers
  hosts: dns_secondary
  serial: 1
  roles:
    - role: pdns_auth
```
