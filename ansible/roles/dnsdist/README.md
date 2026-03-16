# dnsdist Role

Installs and configures dnsdist from the official PowerDNS repository on Debian-based systems.

This role sets up dnsdist as a DNS load balancer and forwarder, routing internal domain queries to authoritative backends and external queries to public resolvers.

## Functionality
- Adds the PowerDNS repository for `dnsdist`.
- Installs the `dnsdist` package.
- Deploys a configuration file (`/etc/dnsdist/dnsdist.conf`) based on a template.
- Enables and starts the `dnsdist` service.

## Variables

All variables are configured in `defaults/main.yml` and are designed to be populated from environment variables using `lookup('ansible.builtin.env', '...')`. See `ansible/.envrc.sample` for a complete list and descriptions.

### Key Environment Variables
- `PRIMARY_AUTH_SERVER`: Address of the primary authoritative DNS server.
- `SECONDARY_AUTH_SERVER`: Address of the secondary authoritative DNS server.
- `DNSDIST_LOCAL_FORWARDER`: Address of a local recursive resolver (e.g., Unbound).
- `DNSDIST_INTERNAL_DOMAIN`: The internal domain to route to authoritative backends (e.g., `example.internal.`).
- `DNSDIST_INTERNAL_CHECK_NAME`: The FQDN to use for health checks against internal backends (e.g., `ns1.example.internal.`).
- `DNSDIST_WEB_PASSWORD`, `DNSDIST_WEB_API_KEY`, `DNSDIST_CONSOLE_KEY`: Credentials for the web UI and console.
- `DNSDIST_MGMT_ACL`, `DNSDIST_CONSOLE_ACL`: IP ranges allowed to access management interfaces.

### Optional Variables
- `dnsdist_server_addr`: The IP address for dnsdist to bind to. Defaults to the host's primary IPv4 address.

## Dependencies
None.

## Usage
This role is typically used within a playbook targeting DNS servers.

```yaml
# In playbooks/dns.yml
- name: Setup DNS Servers
  hosts: dns_primary
  roles:
    - role: dnsdist
```
