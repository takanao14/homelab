# Homelab Ansible Automation

Ansible playbooks and roles for provisioning and configuring homelab infrastructure.

## Directory Structure

```
ansible/
├── ansible.cfg                      # Ansible configuration (SOPS plugin enabled)
├── requirements.yaml                 # Ansible Galaxy collection dependencies
├── inventories/
│   └── homelab/
│       ├── hosts.yaml                        # Inventory (committed, no secrets)
│       ├── group_vars/
│       │   ├── all.yaml                      # Shared non-secret variables
│       │   ├── all.sops.yaml                # SOPS-encrypted global secrets
│       │   ├── dns_primary.yaml
│       │   ├── dns_secondary.yaml
│       │   ├── dhcp.yaml
│       │   └── syslog.yaml
│       └── host_vars/
│           └── <hostname>.sops.yaml         # SOPS-encrypted host-specific secrets (e.g. ansible_user)
├── playbooks/
│   ├── dns.yaml
│   ├── dhcp.yaml
│   ├── syslog.yaml
│   ├── node_exporter.yaml
│   └── proxmox.yaml
└── roles/
    ├── dnsdist/
    ├── dnscollector/
    ├── pdns_auth/
    ├── kea/
    ├── vector/
    └── node_exporter/
```

## Getting Started

### 1. Install Dependencies

Install the required Ansible collections and Python dependencies:

```bash
cd ansible
# Install Ansible collections
ansible-galaxy collection install -r requirements.yaml

# Install Python requirements (required for community.sops)
pip install sops
```

### 2. Set Up Secrets

Secrets are managed with SOPS and loaded natively by Ansible via the `community.sops.sops` vars plugin.

Edit the encrypted files directly:

```bash
# Global secrets
sops edit inventories/homelab/group_vars/all.sops.yaml

# Host-specific secrets (e.g. SSH user)
sops edit inventories/homelab/host_vars/primary01.sops.yaml
```

### 3. Run Playbooks

Ensure your environment is ready (e.g., `SOPS_AGE_KEY` environment variable is set or age key file exists at `~/.config/sops/age/keys.txt`). Ansible will automatically decrypt `.sops.yaml` files during execution.

```bash
# DNS stack
ansible-playbook playbooks/dns.yaml

# DHCP server
ansible-playbook playbooks/dhcp.yaml

# Syslog aggregator
ansible-playbook playbooks/syslog.yaml

# Node Exporter
ansible-playbook playbooks/node_exporter.yaml

# Proxmox maintenance user setup
ansible-playbook playbooks/proxmox.yaml

# Dry run
ansible-playbook playbooks/dns.yaml --check
```

## Playbooks

### DNS (`playbooks/dns.yaml`)
Deploys PowerDNS Authoritative Server and dnsdist.
- **Roles:** `pdns_auth`, `dnsdist`
- **Hosts:** `dns_primary`, `dns_secondary`

### DHCP (`playbooks/dhcp.yaml`)
Deploys Kea DHCPv4 server.
- **Role:** `kea`
- **Hosts:** `dhcp`
- **Config:** `group_vars/dhcp.yaml` (`kea_subnet4` でサブネット・プール・予約を定義)

### Syslog (`playbooks/syslog.yaml`)
Deploys Vector as a syslog aggregator (UDP 514), parses RFC 3164/5424 および非標準フォーマット、Lokiへ転送。
- **Role:** `vector`
- **Hosts:** `syslog`
- **Config:** `group_vars/syslog.yaml` (`vector_config` でsource/transform/sinkを定義)

### Node Exporter (`playbooks/node_exporter.yaml`)
Installs `prometheus-node-exporter` for metrics collection.
- **Role:** `node_exporter`
- **Hosts:** `node_exporter` group

### Proxmox Configuration (`playbooks/proxmox.yaml`)
Creates a maintenance user on Proxmox VE hosts with sudo and Proxmox Administrator privileges.
- **Hosts:** `proxmox`
- **Secrets required:** `MAINTENANCE_USER`, `MAINTENANCE_PASSWORD_HASH`, `SSH_KEY_PATH`

## Secret Variables

| Variable | Used by | Description |
|----------|---------|-------------|
| `MAINTENANCE_USER` | proxmox.yaml | Maintenance username |
| `MAINTENANCE_PASSWORD_HASH` | proxmox.yaml | Hashed password (`openssl passwd -6`) |
| `SSH_KEY_PATH` | proxmox.yaml | Path to SSH public key file |
| `DNSDIST_WEB_PASSWORD` | dnsdist role | dnsdist web UI password |
| `DNSDIST_WEB_API_KEY` | dnsdist role | dnsdist API key |
| `DNSDIST_CONSOLE_KEY` | dnsdist role | dnsdist console key |
| `PDNS_PRIMARY_API_KEY` | pdns_auth role | PowerDNS primary API key |
| `PDNS_SECONDARY_API_KEY` | pdns_auth role | PowerDNS secondary API key |

## Non-Secret Configuration

Shared non-secret variables are in `inventories/homelab/group_vars/all.yaml`:

```yaml
primary_auth_server: "192.168.10.242:1053"
secondary_auth_server: "192.168.10.241:1053"
```

Host-specific non-secret variables (e.g., `pdns_role`, `node_exporter_args`) are defined in their respective group_vars files or the inventory.

## Tips

- Use `--check` for dry runs.
- Ensure your SSH keys are loaded (`ssh-add`) before running playbooks.
- `community.sops.sops` plugin is used to handle encrypted variables. It is enabled via `vars_plugins_enabled` in `ansible.cfg`.
