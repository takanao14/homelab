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
│       │   ├── log_collector.yaml
│       │   └── vector_lxc.yaml       # journald policy for Vector-enabled LXC guests
│       └── host_vars/
│           └── <hostname>.sops.yaml # Host-specific secrets, including PowerDNS API keys
├── playbooks/                          # see "Naming convention" below
│   ├── bootstrap.yaml                   # new-host baseline (imports common-* hygiene)
│   ├── pdns_auth.yaml                   # system (no prefix)
│   ├── dnsdist.yaml
│   ├── caddy.yaml
│   ├── dhcp.yaml
│   ├── forgejo.yaml
│   ├── forgejo_runner.yaml
│   ├── netbox.yaml
│   ├── seaweedfs.yaml
│   ├── log_collector.yaml
│   ├── blackbox_exporter.yaml
│   ├── openbao.yaml
│   ├── proxmox.yaml
│   ├── gpuvm.yaml
│   ├── common-vector.yaml              # cross-cutting (common- prefix)
│   ├── common-journald.yaml
│   ├── common-timezone.yaml
│   ├── common-rsyslog.yaml
│   ├── common-node_exporter.yaml
│   ├── common-chrony.yaml
│   ├── common-apt_mirror.yaml
│   ├── common-unattended_upgrades.yaml
│   ├── common-maintenance_user.yaml
│   ├── common-users.yaml
│   ├── ops-package_upgrade.yaml        # day-2 / operational (ops- prefix)
│   ├── ops-pdns_sync.yaml
│   ├── ops-openbao_bootstrap.yaml
│   ├── ops-openbao_configure.yaml
│   ├── ops-openbao_configure_userpass.yaml
│   └── ops-openbao_seed_secrets.yaml
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
    ├── journald/
    ├── lxc_logging/                     # meta-role: vector + journald bundle
    └── rsyslog/
```

## Naming convention

Playbooks are named by class so the kind is obvious at a glance:

| Class | Prefix | Meaning | Example |
|-------|--------|---------|---------|
| System | none | Build one service (relocatable). A host may be the target of several. | `netbox.yaml`, `seaweedfs.yaml` |
| Cross-cutting | `common-` | A role applied across many systems; the bulk / version-bump entry point, targeting a dedicated group or host-pattern. | `common-vector.yaml`, `common-chrony.yaml` |
| Day-2 / operational | `ops-` | A procedural maintenance action, not idempotent service config. | `ops-package_upgrade.yaml`, `ops-openbao_bootstrap.yaml` |

### What a system playbook embeds

A system playbook embeds only **service-coupled** concerns: the service role
plus the roles whose configuration the service *owns* — i.e. the log-shipping
stack (`vector` + `journald`, via the `lxc_logging` meta-role), since each
service supplies its own `vector_config`. `timezone` is also embedded as a
pragmatic exception (cheap, idempotent, keeps log timestamps correct on a
single-playbook run).

It deliberately does **not** embed fleet-uniform **host hygiene** (`apt_mirror`,
`chrony`, `unattended_upgrades`, `node_exporter`): that config is identical on
every host, so duplicating it into each service playbook would violate DRY. The
test: *does this role's config vary per service?* If no, it belongs to the host
baseline, not the service playbook.

Every cross-cutting role additionally owns a `common-<role>.yaml` playbook so it
can be rolled out fleet-wide in one run.

### Provisioning a new host

Host hygiene is applied at host bring-up, before services, via `bootstrap.yaml`
(a thin aggregate that imports the baseline `common-*` playbooks):

```bash
# 1. Baseline a freshly created host (apt_mirror first, then timezone, chrony,
#    unattended_upgrades, node_exporter). Per-play host patterns + --limit select
#    the applicable subset automatically.
ansible-playbook playbooks/bootstrap.yaml --limit <newhost>

# 2. Deploy the service onto the baselined host.
ansible-playbook playbooks/<system>.yaml --limit <newhost>
```

`bootstrap.yaml` covers only the universal baseline; narrow roles applied to a
few hosts (`rsyslog`, `maintenance_user`) are run via their own `common-*.yaml`.

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

### Known Deprecation Warnings

As of 2026-06-20, syntax checks report that
`ansible.builtin.apt_repository` is deprecated and scheduled for removal in
ansible-core 2.25. The following tasks still use it:

- `roles/dnsdist/tasks/main.yaml`: PowerDNS dnsdist repository
- `roles/pdns_auth/tasks/main.yaml`: PowerDNS authoritative repository

Migrate these tasks to `ansible.builtin.deb822_repository` before upgrading to
ansible-core 2.25. This warning does not currently prevent the playbooks from
passing syntax checks.

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

# Log collector (log1 build)
ansible-playbook playbooks/log_collector.yaml

# Vector agent, all Vector hosts at once (e.g. version bump)
ansible-playbook playbooks/common-vector.yaml

# Node Exporter
ansible-playbook playbooks/common-node_exporter.yaml

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
ansible-playbook playbooks/common-maintenance_user.yaml

# Bulk user accounts on shared VMs
ansible-playbook playbooks/common-users.yaml

# OS package upgrade (all hosts; apt on Debian/Ubuntu, dnf on Rocky/RHEL)
ansible-playbook playbooks/ops-package_upgrade.yaml

# Time synchronization (chrony -> router; physical hosts and VMs, not LXC)
ansible-playbook playbooks/common-chrony.yaml

# Reapply journald policy to all Vector-enabled LXC guests (normally applied by each
# Vector-enabled LXC service playbook)
ansible-playbook playbooks/common-journald.yaml

# Dry run
ansible-playbook playbooks/pdns_auth.yaml --check
```

## Playbooks

| Playbook | Hosts | Class |
|----------|-------|-------|
| `pdns_auth.yaml` | `dns_primary`, `dns_secondary` | system |
| `dnsdist.yaml` | `dnsdist` | system |
| `dhcp.yaml` | `dhcp` | system |
| `caddy.yaml` | `caddy` | system |
| `log_collector.yaml` | `log_collector` | system |
| `blackbox_exporter.yaml` | `blackbox_exporter` | system |
| `forgejo.yaml` | `forgejo` | system |
| `forgejo_runner.yaml` | `forgejo_runner` | system |
| `netbox.yaml` | `netbox` | system |
| `seaweedfs.yaml` | `seaweedfs` | system |
| `openbao.yaml` | `openbao` | system |
| `proxmox.yaml` | `proxmox` | system (platform) |
| `gpuvm.yaml` | `gpuvm` | system |
| `common-vector.yaml` | `vector` | cross-cutting |
| `common-journald.yaml` | `vector_lxc` | cross-cutting |
| `common-timezone.yaml` | `timezone` | cross-cutting |
| `common-rsyslog.yaml` | `rsyslog` | cross-cutting |
| `common-node_exporter.yaml` | `node_exporter` | cross-cutting |
| `common-chrony.yaml` | `all:!lxc` | cross-cutting |
| `common-apt_mirror.yaml` | `all` | cross-cutting |
| `common-unattended_upgrades.yaml` | `all:!proxmox` | cross-cutting |
| `common-maintenance_user.yaml` | `lxc` | cross-cutting |
| `common-users.yaml` | `shared_vms` | cross-cutting |
| `ops-package_upgrade.yaml` | `all:!proxmox` | ops |
| `ops-pdns_sync.yaml` | `dns_primary`, `dns_secondary` | ops |
| `ops-openbao_bootstrap.yaml` | `openbao` | ops |
| `ops-openbao_configure.yaml` | `openbao` | ops |
| `ops-openbao_configure_userpass.yaml` | `openbao` | ops |
| `ops-openbao_seed_secrets.yaml` | `openbao` | ops |

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
| `openbao_secrets` | `group_vars/openbao.sops.yaml` | Application secrets seeded into OpenBao KV, including the Alertmanager Discord webhook |
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
