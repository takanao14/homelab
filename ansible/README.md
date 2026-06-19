# Homelab Ansible Automation

Ansible playbooks and roles for provisioning and configuring homelab infrastructure.

## Directory Structure

```
ansible/
├── .ansible-lint                    # Lint exclusions for generated/vendor files
├── ansible.cfg                      # Ansible configuration (SOPS plugin enabled)
├── requirements.yaml                # Ansible Galaxy collection dependencies
├── collections/                     # Locally installed collections (gitignored)
├── inventories/
│   └── homelab/
│       ├── hosts.yaml               # Inventory (no secrets)
│       ├── group_vars/
│       │   ├── all.yaml             # Shared non-secret variables
│       │   ├── dns.yaml             # Shared DNS variables (primary/secondary server addresses)
│       │   ├── dns_auth.yaml        # pdns_auth group variables
│       │   ├── dns_primary.yaml     # Primary-specific pdns_auth variables
│       │   ├── dns_secondary.yaml   # Secondary-specific pdns_auth variables
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
│       │   ├── openbao.yaml
│       │   ├── openbao.sops.yaml
│       │   ├── proxmox.sops.yaml
│       │   ├── seaweedfs.yaml
│       │   ├── seaweedfs.sops.yaml
│       │   └── syslog.yaml
│       └── host_vars/
│           └── <hostname>.sops.yaml # Host-specific secrets, including PowerDNS API keys
├── playbooks/
│   ├── pdns_auth.yaml
│   ├── pdns_sync.yaml
│   ├── dnsdist.yaml
│   ├── caddy.yaml
│   ├── dhcp.yaml
│   ├── forgejo.yaml
│   ├── forgejo_runner.yaml
│   ├── netbox.yaml
│   ├── seaweedfs.yaml
│   ├── node_exporter.yaml
│   ├── blackbox_exporter.yaml
│   ├── syslog.yaml
│   ├── openbao.yaml
│   ├── openbao_bootstrap.yaml
│   ├── openbao_configure.yaml
│   ├── openbao_configure_userpass.yaml
│   ├── openbao_seed_secrets.yaml
│   ├── proxmox.yaml
│   ├── maintenance_user.yaml
│   ├── package_upgrade.yaml
│   ├── apt_mirror.yaml
│   ├── chrony.yaml
│   ├── unattended_upgrades.yaml
│   ├── users.yaml
│   ├── gpuvm.yaml
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
    ├── seaweedfs/
    ├── node_exporter/
    ├── blackbox_exporter/
    ├── openbao/
    ├── apt_mirror/
    ├── chrony/
    ├── unattended_upgrades/
    ├── maintenance_user/
    ├── rocm/
    ├── sysctl/
    ├── timezone/
    ├── users/
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
ansible-galaxy collection install -r requirements.yaml -p collections --force
```

Collections are installed under `ansible/collections/` so both Ansible and
`ansible-lint` resolve the same pinned dependencies regardless of how the tools
were installed.

Run the project lint checks from this directory:

```bash
ansible-lint
```

The local Collection installation directory and SOPS-generated encrypted YAML
files are excluded by `.ansible-lint`.

### 2. Set Up Secrets

Secrets are managed with SOPS and loaded natively by Ansible via the `community.sops.sops` vars plugin.

Edit the encrypted files directly:

```bash
# Group-level secrets
sops edit inventories/homelab/group_vars/dnsdist.sops.yaml

# Host-specific secrets (e.g. PowerDNS API key)
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

# SeaweedFS (standalone object storage for Terraform state)
ansible-playbook playbooks/seaweedfs.yaml

# Forgejo
ansible-playbook playbooks/forgejo.yaml

# Forgejo Runner
ansible-playbook playbooks/forgejo_runner.yaml

# OpenBao
ansible-playbook playbooks/openbao.yaml

# Proxmox maintenance user setup
ansible-playbook playbooks/proxmox.yaml

# Maintenance user on LXC containers
ansible-playbook playbooks/maintenance_user.yaml

# Bulk user accounts on shared VMs
ansible-playbook playbooks/users.yaml

# OS package upgrade (all hosts; apt on Debian/Ubuntu, dnf on Rocky/RHEL)
ansible-playbook playbooks/package_upgrade.yaml

# Time synchronization (chrony -> router; physical hosts and VMs, not LXC)
ansible-playbook playbooks/chrony.yaml

# Dry run
ansible-playbook playbooks/pdns_auth.yaml --check
```

## Playbooks

| Playbook | Hosts |
|----------|-------|
| `pdns_auth.yaml` | `dns_primary`, `dns_secondary` |
| `pdns_sync.yaml` | `dns_primary`, `dns_secondary` |
| `dnsdist.yaml` | `dnsdist` |
| `dhcp.yaml` | `dhcp` |
| `caddy.yaml` | `caddy` |
| `syslog.yaml` | `syslog` |
| `node_exporter.yaml` | `node_exporter` |
| `blackbox_exporter.yaml` | `blackbox_exporter` |
| `forgejo.yaml` | `forgejo` |
| `forgejo_runner.yaml` | `forgejo_runner` |
| `netbox.yaml` | `netbox` |
| `seaweedfs.yaml` | `seaweedfs` |
| `openbao.yaml` | `openbao` |
| `openbao_bootstrap.yaml` | `openbao` |
| `openbao_configure.yaml` | `openbao` |
| `openbao_configure_userpass.yaml` | `openbao` |
| `openbao_seed_secrets.yaml` | `openbao` |
| `proxmox.yaml` | `proxmox` |
| `maintenance_user.yaml` | `lxc` |
| `users.yaml` | `shared_vms` |
| `package_upgrade.yaml` | `all:!proxmox` |
| `apt_mirror.yaml` | `all` |
| `unattended_upgrades.yaml` | `all:!proxmox` |
| `chrony.yaml` | `all:!lxc` |
| `gpuvm.yaml` | `gpuvm` |
| `rpi3.yaml` | `rpi3` |

## Secret Variables

| Variable | Sops file | Description |
|----------|-----------|-------------|
| `PDNS_PRIMARY_API_KEY` | `host_vars/ns1.sops.yaml` | PowerDNS primary API key |
| `PDNS_SECONDARY_API_KEY` | `host_vars/ns2.sops.yaml`, `host_vars/ns3.sops.yaml` | Per-secondary PowerDNS API key |
| `DNSDIST_WEB_PASSWORD` | `group_vars/dnsdist.sops.yaml` | dnsdist web UI password |
| `DNSDIST_WEB_API_KEY` | `group_vars/dnsdist.sops.yaml` | dnsdist API key |
| `DNSDIST_CONSOLE_KEY` | `group_vars/dnsdist.sops.yaml` | dnsdist console key |
| `MAINTENANCE_USER` | `group_vars/proxmox.sops.yaml` | Proxmox maintenance username |
| `MAINTENANCE_PASSWORD_HASH` | `group_vars/proxmox.sops.yaml` | Hashed password (`openssl passwd -6`) |
| `SSH_KEY_PATH` | `group_vars/proxmox.sops.yaml` | Path to SSH public key file |
| `netbox_db_password` | `group_vars/netbox.sops.yaml` | NetBox PostgreSQL password |
| `netbox_secret_key` | `group_vars/netbox.sops.yaml` | NetBox Django secret key |
| `netbox_superuser_password` | `group_vars/netbox.sops.yaml` | NetBox superuser password |
| `openbao_seal_key` | `group_vars/openbao.sops.yaml` | OpenBao static seal key (base64-encoded 32 bytes) |
| `openbao_root_token` | `group_vars/openbao.sops.yaml` | OpenBao root token (emergency backup) |
| `openbao_admin_token` | `group_vars/openbao.sops.yaml` | OpenBao admin token for configuration |
| `openbao_k8s_token_reviewer_jwt` | `group_vars/openbao.sops.yaml` | Kubernetes token reviewer ServiceAccount JWT |
| `openbao_k8s_ca_cert` | `group_vars/openbao.sops.yaml` | PEM CA certificate of the Kubernetes cluster |
| `seaweedfs_s3_access_key` | `group_vars/seaweedfs.sops.yaml` | SeaweedFS S3 access key for the Terraform identity |
| `seaweedfs_s3_secret_key` | `group_vars/seaweedfs.sops.yaml` | SeaweedFS S3 secret key for the Terraform identity |
| `seaweedfs_admin_password` | `group_vars/seaweedfs.sops.yaml` | SeaweedFS admin UI password (empty = auth disabled) |
| `users_accounts` | `group_vars/shared_vms.sops.yaml` (create when enabling `shared_vms`) | List of `{name, password}` accounts created by `users.yaml` |

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
