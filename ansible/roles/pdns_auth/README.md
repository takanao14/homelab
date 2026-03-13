# PowerDNS Authoritative Role (pdns_auth)

Installs and configures PowerDNS Authoritative Server with an SQLite3 backend on Debian-based systems.

This role can configure the server as either a `primary` (master) or `secondary` (slave) instance.

## Functionality
- Adds the official PowerDNS repository for `pdns-auth`.
- Installs `pdns-server`, `pdns-backend-sqlite3`, and `sqlite3`.
- Initializes the SQLite database if it doesn't exist.
- Deploys a configuration file (`/etc/powerdns/pdns.conf`) based on the specified role (`primary` or `secondary`).
- Enables and starts the `pdns` service.

## Variables

### Role-Defining Variable
- `pdns_role`: Must be set to either `primary` or `secondary`. This is typically done in `group_vars`.
  ```yaml
  # In inventories/homelab/group_vars/dns_primary.yml
  pdns_role: primary
  ```

### Environment Variables
All other variables are configured in `defaults/main.yml` and are designed to be populated from environment variables using `lookup('ansible.builtin.env', '...')`. See `ansible/.envrc.sample` for a complete list and descriptions.

#### Key Environment Variables
- `PRIMARY_AUTH_SERVER`: Address of the primary server (used by both roles).
- `SECONDARY_AUTH_SERVER`: Address of the secondary server (used by both roles).
- `PDNS_PRIMARY_API_KEY`: API key for the primary server.
- `PDNS_SECONDARY_API_KEY`: API key for the secondary server.
- `PDNS_PRIMARY_ALLOW_AXFR_IPS`: IP(s) allowed to perform zone transfers from the primary (should be the secondary's IP).
- `PDNS_PRIMARY_ALSO_NOTIFY`: IP(s) to send NOTIFY packets to when a zone on the primary changes.
- `PDNS_WEBSERVER_PORT`: Port for the webserver/API.
- `PDNS_WEBSERVER_ALLOW_FROM`: IP range(s) allowed to access the webserver/API.

## Dependencies
None.

## Usage
This role is typically used within a playbook targeting DNS servers, with the `pdns_role` variable differentiating the behavior.

```yaml
# In playbooks/dns.yml
- name: Setup Primary DNS Server
  hosts: dns_primary
  roles:
    - role: pdns_auth

- name: Setup Secondary DNS Servers
  hosts: dns_secondary
  roles:
    - role: pdns_auth
```
