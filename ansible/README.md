# Homelab Ansible Automation

Ansible playbooks and roles for provisioning and configuring homelab infrastructure.

## Directory Structure

```
ansible/
├── ansible.cfg
├── secrets.enc.env                          # SOPS-encrypted secrets (committed)
├── .envrc                                   # Decrypts secrets and inventory (gitignored)
├── inventories/
│   └── homelab/
│       ├── hosts.enc.yml                    # SOPS-encrypted inventory (committed)
│       ├── hosts.yml                        # Decrypted inventory (gitignored, auto-generated)
│       └── group_vars/
│           ├── all.yml                      # Shared non-secret variables
│           ├── dns_primary.yml
│           ├── dns_secondary.yml
│           ├── dhcp.yml
│           └── syslog.yml
├── playbooks/
│   ├── dns.yml
│   ├── dhcp.yml
│   ├── syslog.yml
│   ├── node_exporter.yml
│   └── proxmox.yml
└── roles/
    ├── dnsdist/
    ├── dnscollector/
    ├── pdns_auth/
    ├── kea/
    ├── vector/
    └── node_exporter/
```

## Getting Started

### 1. Set Up Secrets

Secrets are managed with SOPS. Edit the encrypted files directly:

```bash
cd ansible
sops edit secrets.enc.env          # API keys, passwords
sops edit inventories/homelab/hosts.enc.yml   # SSH usernames, hosts
```

### 2. Load Environment Variables

The `.envrc` decrypts secrets and auto-generates `hosts.yml` from the encrypted inventory:

```bash
direnv allow   # first time only; re-run after .envrc changes
```

On each directory entry, `direnv` will:
1. Decrypt `secrets.enc.env` and export the variables.
2. Decrypt `inventories/homelab/hosts.enc.yml` → `inventories/homelab/hosts.yml`.

### 3. Run Playbooks

```bash
# DNS stack
ansible-playbook playbooks/dns.yml

# DHCP server
ansible-playbook playbooks/dhcp.yml

# Syslog aggregator
ansible-playbook playbooks/syslog.yml

# Node Exporter
ansible-playbook playbooks/node_exporter.yml

# Proxmox maintenance user setup
ansible-playbook playbooks/proxmox.yml

# Dry run
ansible-playbook playbooks/dns.yml --check
```

## Playbooks

### DNS (`playbooks/dns.yml`)
Deploys PowerDNS Authoritative Server and dnsdist.
- **Roles:** `pdns_auth`, `dnsdist`
- **Hosts:** `dns_primary`, `dns_secondary`

### DHCP (`playbooks/dhcp.yml`)
Deploys Kea DHCPv4 server.
- **Role:** `kea`
- **Hosts:** `dhcp`
- **Config:** `group_vars/dhcp.yml` (`kea_subnet4` でサブネット・プール・予約を定義)

### Syslog (`playbooks/syslog.yml`)
Deploys Vector as a syslog aggregator (UDP 514), parses RFC 3164/5424 および非標準フォーマット、Lokiへ転送。
- **Role:** `vector`
- **Hosts:** `syslog`
- **Config:** `group_vars/syslog.yml` (`vector_config` でsource/transform/sinkを定義)

### Node Exporter (`playbooks/node_exporter.yml`)
Installs `prometheus-node-exporter` for metrics collection.
- **Role:** `node_exporter`
- **Hosts:** `node_exporter` group

### Proxmox Configuration (`playbooks/proxmox.yml`)
Creates a maintenance user on Proxmox VE hosts with sudo and Proxmox Administrator privileges.
- **Hosts:** `proxmox`
- **Secrets required:** `MAINTENANCE_USER`, `MAINTENANCE_PASSWORD_HASH`, `SSH_KEY_PATH`

## Secret Variables

| Variable | Used by | Description |
|----------|---------|-------------|
| `MAINTENANCE_USER` | proxmox.yml | Maintenance username |
| `MAINTENANCE_PASSWORD_HASH` | proxmox.yml | Hashed password (`openssl passwd -6`) |
| `SSH_KEY_PATH` | proxmox.yml | Path to SSH public key file |
| `DNSDIST_WEB_PASSWORD` | dnsdist role | dnsdist web UI password |
| `DNSDIST_WEB_API_KEY` | dnsdist role | dnsdist API key |
| `DNSDIST_CONSOLE_KEY` | dnsdist role | dnsdist console key |
| `PDNS_PRIMARY_API_KEY` | pdns_auth role | PowerDNS primary API key |
| `PDNS_SECONDARY_API_KEY` | pdns_auth role | PowerDNS secondary API key |

## Non-Secret Configuration

Shared non-secret variables are in `inventories/homelab/group_vars/all.yml`:

```yaml
primary_auth_server: "192.168.10.242:1053"
secondary_auth_server: "192.168.10.241:1053"
```

Host-specific non-secret variables (e.g., `pdns_role`, `node_exporter_args`) are defined in their respective group_vars files or the inventory.

## Tips

- Use `--check` for dry runs.
- Ensure your SSH keys are loaded (`ssh-add`) before running playbooks.
- `hosts.yml` is gitignored; it is regenerated from `hosts.enc.yml` by `.envrc` on each `direnv allow`.
