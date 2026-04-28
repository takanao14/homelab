# Homelab Ansible Automation

Ansible playbooks and roles for provisioning and configuring homelab infrastructure.

## Directory Structure

```
ansible/
├── ansible.cfg                      # Ansible configuration (SOPS plugin enabled)
├── requirements.yaml                 # Ansible Galaxy collection dependencies
├── inventories/
│   └── homelab/
│       ├── hosts.yaml               # Inventory (no secrets)
│       ├── group_vars/
│       │   ├── all.yaml             # Shared non-secret variables
│       │   ├── dns.yaml             # Shared DNS variables (primary/secondary server addresses)
│       │   ├── dns_auth.yaml        # pdns_auth group variables
│       │   ├── dns_primary.yaml     # Primary-specific pdns_auth variables
│       │   ├── dns_primary.sops.yaml
│       │   ├── dns_secondary.yaml   # Secondary-specific pdns_auth variables
│       │   ├── dns_secondary.sops.yaml
│       │   ├── dnsdist.yaml
│       │   ├── dnsdist.sops.yaml
│       │   ├── caddy.yaml
│       │   ├── caddy.sops.yaml
│       │   ├── dhcp.sops.yaml
│       │   ├── forgejo.yaml
│       │   ├── forgejo.sops.yaml
│       │   ├── forgejo_runner.sops.yaml
│       │   ├── lxc.sops.yaml
│       │   ├── netbox.yaml
│       │   ├── netbox.sops.yaml
│       │   ├── node_exporter.yaml
│       │   ├── node_exporter_rpi.yaml
│       │   ├── proxmox.sops.yaml
│       │   └── syslog.yaml
│       └── host_vars/
│           └── <hostname>.sops.yaml # SOPS-encrypted host-specific secrets (e.g. ansible_user)
├── playbooks/
│   ├── pdns_auth.yaml
│   ├── dnsdist.yaml
│   ├── caddy.yaml
│   ├── dhcp.yaml
│   ├── forgejo.yaml
│   ├── forgejo_runner.yaml
│   ├── netbox.yaml
│   ├── node_exporter.yaml
│   ├── blackbox_exporter.yaml
│   ├── syslog.yaml
│   ├── proxmox.yaml
│   ├── maintenance_user.yaml
│   ├── apt_upgrade.yaml
│   ├── apt_mirror.yaml
│   ├── unattended_upgrades.yaml
│   ├── gpuvm.yaml
│   ├── lemonade.yaml
│   └── rpi3.yaml
└── roles/
    ├── pdns_auth/
    ├── dnsdist/
    ├── dnscollector/
    ├── caddy/
    ├── vector/
    ├── kea/
    ├── forgejo/
    ├── forgejo_runner/
    ├── netbox/
    ├── node_exporter/
    ├── blackbox_exporter/
    ├── apt_mirror/
    ├── unattended_upgrades/
    ├── maintenance_user/
    ├── lemonade/
    ├── rocm/
    ├── timezone/
    └── rsyslog/
```

## Getting Started

### 1. Install Dependencies

Install the SOPS binary and the required Ansible collections:

```bash
# Install SOPS binary (macOS)
brew install sops

# Install Ansible collections
cd ansible
ansible-galaxy collection install -r requirements.yaml
```

### 2. Set Up Secrets

Secrets are managed with SOPS and loaded natively by Ansible via the `community.sops.sops` vars plugin.

Edit the encrypted files directly:

```bash
# Group-level secrets (e.g. PowerDNS primary API key)
sops edit inventories/homelab/group_vars/dns_primary.sops.yaml

# Host-specific secrets (e.g. SSH user)
sops edit inventories/homelab/host_vars/ns1.sops.yaml
```

### 3. Run Playbooks

Ensure your environment is ready (e.g., `SOPS_AGE_KEY` environment variable is set or age key file exists at `~/.config/sops/age/keys.txt`). Ansible will automatically decrypt `.sops.yaml` files during execution.

```bash
# PowerDNS Authoritative Server
ansible-playbook playbooks/pdns_auth.yaml

# dnsdist
ansible-playbook playbooks/dnsdist.yaml

# DHCP server
ansible-playbook playbooks/dhcp.yaml

# Syslog aggregator
ansible-playbook playbooks/syslog.yaml

# Node Exporter
ansible-playbook playbooks/node_exporter.yaml

# Forgejo
ansible-playbook playbooks/forgejo.yaml

# Forgejo Runner
ansible-playbook playbooks/forgejo_runner.yaml

# Proxmox maintenance user setup
ansible-playbook playbooks/proxmox.yaml

# Dry run
ansible-playbook playbooks/pdns_auth.yaml --check
```

## Playbooks

### PowerDNS Authoritative Server (`playbooks/pdns_auth.yaml`)
Deploys PowerDNS Authoritative Server with SQLite backend.
- **Role:** `pdns_auth`
- **Hosts:** `dns_primary` (ns1), `dns_secondary` (ns2)
- **Config:** `group_vars/dns_auth.yaml`, `group_vars/dns_primary.yaml`, `group_vars/dns_secondary.yaml`

### dnsdist (`playbooks/dnsdist.yaml`)
Deploys dnsdist as a DNS load balancer / front-end.
- **Role:** `dnsdist`
- **Hosts:** `dnsdist` (dist1, dist2)
- **Config:** `group_vars/dnsdist.yaml`

### DHCP (`playbooks/dhcp.yaml`)
Deploys Kea DHCPv4 server.
- **Role:** `kea`
- **Hosts:** `dhcp`
- **Config:** `group_vars/dhcp.sops.yaml` (define subnets, pools, and reservations via `kea_subnet4`)

### Caddy (`playbooks/caddy.yaml`)
Deploys Caddy as a reverse proxy.
- **Role:** `caddy`
- **Hosts:** `caddy`
- **Config:** `group_vars/caddy.yaml`

### Syslog (`playbooks/syslog.yaml`)
Deploys Vector as a syslog aggregator (UDP 514), parses RFC 3164/5424 and non-standard formats, and forwards to Loki.
- **Role:** `vector`
- **Hosts:** `syslog`
- **Config:** `group_vars/syslog.yaml` (define sources, transforms, and sinks via `vector_config`)

### Node Exporter (`playbooks/node_exporter.yaml`)
Installs `prometheus-node-exporter` for metrics collection.
- **Role:** `node_exporter`
- **Hosts:** `node_exporter` group (includes `node_exporter_rpi` subgroup for Raspberry Pi hosts)
- **Config:**
  - `group_vars/node_exporter.yaml` (`node_exporter_base_args`: common args for all hosts)
  - `group_vars/node_exporter_rpi.yaml` (`node_exporter_rpi_args`: RPi-specific args)

### Forgejo (`playbooks/forgejo.yaml`)
Deploys Forgejo self-hosted Git service.
- **Role:** `forgejo`
- **Hosts:** `forgejo`

### Forgejo Runner (`playbooks/forgejo_runner.yaml`)
Deploys Forgejo Actions Runner.
- **Role:** `forgejo_runner`
- **Hosts:** `forgejo_runner`

### NetBox (`playbooks/netbox.yaml`)
Deploys NetBox IPAM/DCIM.
- **Role:** `netbox`
- **Hosts:** `netbox`
- **Config:** `group_vars/netbox.yaml`

### Proxmox Configuration (`playbooks/proxmox.yaml`)
Creates a maintenance user on Proxmox VE hosts with sudo and Proxmox Administrator privileges.
- **Hosts:** `proxmox`
- **Secrets required:** `MAINTENANCE_USER`, `MAINTENANCE_PASSWORD_HASH`, `SSH_KEY_PATH`

### Raspberry Pi 3 (`playbooks/rpi3.yaml`)
Configures rsyslog on Raspberry Pi 3.
- **Role:** `rsyslog`
- **Hosts:** `rpi3`

## Secret Variables

| Variable | Sops file | Description |
|----------|-----------|-------------|
| `PDNS_PRIMARY_API_KEY` | `group_vars/dns_primary.sops.yaml` | PowerDNS primary API key |
| `PDNS_SECONDARY_API_KEY` | `group_vars/dns_secondary.sops.yaml` | PowerDNS secondary API key |
| `DNSDIST_WEB_PASSWORD` | `group_vars/dnsdist.sops.yaml` | dnsdist web UI password |
| `DNSDIST_WEB_API_KEY` | `group_vars/dnsdist.sops.yaml` | dnsdist API key |
| `DNSDIST_CONSOLE_KEY` | `group_vars/dnsdist.sops.yaml` | dnsdist console key |
| `MAINTENANCE_USER` | `group_vars/proxmox.sops.yaml` | Proxmox maintenance username |
| `MAINTENANCE_PASSWORD_HASH` | `group_vars/proxmox.sops.yaml` | Hashed password (`openssl passwd -6`) |
| `SSH_KEY_PATH` | `group_vars/proxmox.sops.yaml` | Path to SSH public key file |

## Non-Secret Configuration

DNS backend server addresses are defined in `inventories/homelab/group_vars/dns.yaml` and shared across `pdns_auth` and `dnsdist` roles:

```yaml
primary_auth_server: "192.168.10.233:53"
secondary_auth_server: "192.168.10.234:53"
```

## Tips

- Use `--check` for dry runs.
- Ensure your SSH keys are loaded (`ssh-add`) before running playbooks.
- `community.sops.sops` plugin is used to handle encrypted variables. It is enabled via `vars_plugins_enabled` in `ansible.cfg`.
