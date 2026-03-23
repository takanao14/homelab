# dnsdist Role

Installs and configures dnsdist from the official PowerDNS repository on Debian-based systems.

Sets up dnsdist as a DNS load balancer and forwarder: internal domain queries are routed to PowerDNS Authoritative backends, external queries go to public resolvers.

## Functionality

- Adds the PowerDNS APT repository for `dnsdist`.
- Installs the `dnsdist` package.
- Deploys `/etc/dnsdist/dnsdist.conf` from a Jinja2 template.
- Enables and starts the `dnsdist` service.

## Variables

### Secrets (from SOPS-encrypted `secrets.enc.env`)

Loaded via `lookup('ansible.builtin.env', ...)`. Set these in `ansible/secrets.enc.env`.

| Variable | Description |
|----------|-------------|
| `DNSDIST_WEB_PASSWORD` | Web UI password |
| `DNSDIST_WEB_API_KEY` | Web UI API key |
| `DNSDIST_CONSOLE_KEY` | Console key |

### Non-secret variables (in `defaults/main.yml`)

These values are hardcoded in `defaults/main.yml`. Override per-host or per-group as needed.

| Variable | Default | Description |
|----------|---------|-------------|
| `dnsdist_local_forwarder` | `192.168.10.1:53` | Local recursive resolver |
| `dnsdist_mgmt_acl` | `127.0.0.1/32,192.168.0.0/16,10.0.0.0/8` | ACL for management access |
| `dnsdist_console_acl` | `127.0.0.1/32` | ACL for console access |
| `dnsdist_internal_domains` | `home.butaco.net.`, `prd.butaco.net.`, `dev.butaco.net.` | Domains routed to auth backends |
| `dnsdist_internal_check_name` | `ns1.home.butaco.net.` | Health check FQDN |
| `dnsdist_server_addr` | Host primary IPv4 | IP address for dnsdist to bind to |

### Shared variables (from `group_vars/all.yml`)

| Variable | Description |
|----------|-------------|
| `primary_auth_server` | Address of the primary authoritative DNS server (`host:port`) |
| `secondary_auth_server` | Address of the secondary authoritative DNS server (`host:port`) |

## Dependencies

None.

## Usage

```yaml
# In playbooks/dns.yml
- name: Setup DNS Servers
  hosts: dns_primary
  roles:
    - role: dnsdist
```
