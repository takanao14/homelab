# Homelab Ansible Automation

Ansible playbooks and roles for provisioning and configuring homelab infrastructure.

## Directory Structure

```text
ansible/
├── ansible.cfg        # SOPS vars plugin and local collection paths
├── requirements.yaml # Collection dependencies
├── inventories/homelab/
│   ├── hosts.yaml     # Managed hosts and groups
│   ├── group_vars/    # Shared settings and encrypted secrets
│   └── host_vars/     # Host overrides and encrypted secrets
├── playbooks/
│   ├── bootstrap.yaml # Host baseline aggregate
│   ├── services/      # Service deployment and configuration
│   ├── common/        # Cross-cutting settings
│   ├── ops/           # Operational procedures
│   └── tasks/         # Shared task files
└── roles/             # Reusable roles with individual READMEs
```

## Naming convention

Playbooks are grouped by class. OpenBao operations use a service subdirectory
for their multiple entry points:

| Class | Directory | Meaning | Example |
|-------|--------|---------|---------|
| System | `services/` | Build one service (relocatable). A host may be the target of several. | `playbooks/services/netbox.yaml`, `playbooks/services/seaweedfs.yaml` |
| Cross-cutting | `common/` | A role applied across many systems; the bulk / version-bump entry point, targeting a dedicated group or host-pattern. | `playbooks/common/vector.yaml`, `playbooks/common/chrony.yaml` |
| Day-2 / operational | `ops/` | A procedural maintenance action, not idempotent service config. | `playbooks/ops/package_upgrade.yaml`, `playbooks/ops/openbao/bootstrap.yaml` |

### What a system playbook embeds

Service playbooks include the service role, its log-shipping roles (`vector`
and `journald` through `lxc_logging`), and `timezone` for consistent timestamps.
Each service defines its own `vector_journald_units`. OpenClaw and Hermes also
include `mcp_client` for their SOPS-managed caller credentials.

Fleet-wide settings (`apt_mirror`, `chrony`, `unattended_upgrades`,
`node_exporter`) belong to the host baseline. Cross-cutting roles have
`common/<role>.yaml` entry points for fleet updates.

### Downloading release artifacts

`get_url` defaults to a 10s socket timeout, which aborts mid-transfer on the
multi-MB release archives once egress leaves a SimpleZone. Every `get_url` task
therefore carries `retries: 3` / `delay: 5`, and artifact downloads (archives,
binaries, `.deb`) also raise `timeout` to 60s. The role builds Caddy on the controller
with pinned xcaddy inputs (ADR-0068). Repository signing keys are a few KB and
keep the default timeout. Tasks deliberately omit `until`: with `retries` set and
no `until`, Ansible retries until the task stops failing.

### Provisioning a new host

Apply the host baseline before deploying services with `bootstrap.yaml`
(a thin aggregate that imports the baseline `common/` playbooks):

```bash
# 1. Apply the host baseline.
ansible-playbook playbooks/bootstrap.yaml --limit <newhost>

# 2. Deploy the service onto the baselined host.
ansible-playbook playbooks/services/<system>.yaml --limit <newhost>
```

Without `--limit`, bootstrap targets the full matching inventory, including
hosts that are currently powered off.

Network changes (`netplan`, `networkmanager`) and shared-VM user management
(`users`) remain separate `common/` operations. Each common playbook can
also be run independently for fleet updates.

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

Install collections under `ansible/collections/` so both Ansible and
`ansible-lint` resolve the same pinned dependencies regardless of how the tools
were installed.

Run the project lint checks from this directory:

```bash
ansible-lint
```

`.ansible-lint` excludes the local collection installation directory and
SOPS-generated encrypted YAML files.

### 2. Set Up Secrets

Keep inventory secrets encrypted with SOPS. Ansible loads them through the
`community.sops.sops` vars plugin.

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
ansible-playbook playbooks/services/<system>.yaml --limit <host> --check --diff
ansible-playbook playbooks/services/<system>.yaml --limit <host>
```

Use the [Playbooks](#playbooks) index to choose an entry point. Service-specific
prerequisites and procedures live in the corresponding role README.

Package upgrades run serially on DNS hosts and k0s nodes. Workers are drained
before reboot and uncordoned only after LocalPV is mounted and, where NFS is
enabled, a probe Pod on that worker successfully mounts the export. The
`dns`, `k8s`, and `others` phases can be resumed independently.
The on-demand image builder is excluded from bulk upgrades. Start it in
maintenance mode and use `ops/imagebuilder_maintenance.yaml` instead; see the
[imagebuilder role](roles/imagebuilder/README.md).

Shutdown phase tags are `workloads`, `k8s`, `guests`, and `hypervisors`, in that
order. Startup uses the same tags to wait for hypervisors, start guests, check
cluster readiness, and restore workloads.
Startup ensures cluster guests are running, waits for dependencies, and
restores workloads through Argo CD. NFS-enabled clusters must pass a
kubelet-backed mount probe before GitOps resumes. Environment tags cover all
cluster phases and leave hypervisors running; no `--limit` is needed.

```bash
# Upgrade OS packages, or resume one phase.
ansible-playbook playbooks/ops/package_upgrade.yaml
ansible-playbook playbooks/ops/package_upgrade.yaml --tags others

# Stop workloads; omit --tags to run all shutdown phases.
ansible-playbook playbooks/ops/shutdown.yaml --tags workloads

# Recover after power is restored.
ansible-playbook playbooks/ops/startup.yaml

# Stop and restore one cluster.
ansible-playbook playbooks/ops/shutdown.yaml --tags sandbox
ansible-playbook playbooks/ops/startup.yaml --tags sandbox
ansible-playbook playbooks/ops/shutdown.yaml --tags prd
ansible-playbook playbooks/ops/startup.yaml --tags prd

# Audit Renovate-managed component versions (read-only).
ansible-playbook playbooks/ops/version_audit.yaml

# Preview an OpenBao upgrade before the operator applies it.
ansible-playbook playbooks/ops/openbao/upgrade.yaml --limit openbao --check --diff
```

### Proxmox SimpleZone Static Routes

[The role README](roles/proxmox_simplezone_routes/README.md#apply-flow) covers
preview, manual network reload, verification, and rollback. Ansible only writes
the route file; review the diff before running `ifreload -a` on the target.

### OpenBao operations

Use the OpenBao role README for:

- [Operational authentication and ESO setup](roles/openbao/README.md#kubernetes-eso-integration-setup)
- [Cluster registration after rebuild](roles/openbao/README.md#4-register-each-cluster-with-openbao)
- [Host recovery](roles/openbao/README.md#rebuilding-the-host)

## Playbooks

| Playbook | Hosts | Class |
|----------|-------|-------|
| `playbooks/services/pdns_auth.yaml` | `dns_primary`, `dns_secondary` | system |
| `playbooks/services/dnsdist.yaml` | `dnsdist` | system |
| `playbooks/services/knot-resolver.yaml` | `dns_resolver` | system |
| `playbooks/services/dhcp.yaml` | `dhcp` | system |
| `playbooks/services/dhcp_lease_observer.yaml` | `dhcp_lease_observer` | system |
| `playbooks/services/caddy.yaml` | `caddy` | system |
| `playbooks/services/log_collector.yaml` | `log_collector` | system |
| `playbooks/services/blackbox_exporter.yaml` | `blackbox_exporter` | system |
| `playbooks/services/forgejo.yaml` | `forgejo` | system |
| `playbooks/services/forgejo_runner.yaml` | `forgejo_runner` | system |
| `playbooks/services/netbox.yaml` | `netbox` | system |
| `playbooks/services/pdm.yaml` | `pdm` | system |
| `playbooks/services/lldap.yaml` | `lldap` | system |
| `playbooks/services/authelia.yaml` | `authelia` | system |
| `playbooks/services/seaweedfs.yaml` | `seaweedfs` | system |
| `playbooks/services/openbao.yaml` | `openbao` | system |
| `playbooks/services/proxmox.yaml` | `proxmox` | system (platform) |
| `playbooks/services/gpuvm.yaml` | `gpuvm` | system |
| `playbooks/services/code_server.yaml` | `code_server` | system |
| `playbooks/services/meshcentral.yaml` | `meshcentral` | system |
| `playbooks/services/sssd.yaml` | `sssd` | system |
| `playbooks/services/openclaw.yaml` | `openclaw` | system |
| `playbooks/services/hermes.yaml` | `hermes` | system |
| `playbooks/common/vector.yaml` | `vector` | cross-cutting |
| `playbooks/common/journald.yaml` | `vector_lxc` | cross-cutting |
| `playbooks/common/timezone.yaml` | `timezone` | cross-cutting |
| `playbooks/common/rsyslog.yaml` | `rsyslog` | cross-cutting |
| `playbooks/common/node_exporter.yaml` | `node_exporter` | cross-cutting |
| `playbooks/common/chrony.yaml` | `all:!lxc:!power_only` | cross-cutting |
| `playbooks/common/apt_mirror.yaml` | `all:!power_only` | cross-cutting |
| `playbooks/common/unattended_upgrades.yaml` | `all:!proxmox:!power_only` | cross-cutting |
| `playbooks/common/k8s-storage-client.yaml` | `prd_k8s:sandbox_k8s` | cross-cutting |
| `playbooks/common/maintenance_user.yaml` | `lxc` | cross-cutting |
| `playbooks/common/ssh_authorized_keys.yaml` | `lxc:vm` | cross-cutting |
| `playbooks/common/users.yaml` | `shared_vms` | cross-cutting |
| `playbooks/common/netplan.yaml` | `netplan` | cross-cutting |
| `playbooks/common/networkmanager.yaml` | `networkmanager` | cross-cutting |
| `playbooks/ops/package_upgrade.yaml` | `dns` (serial), k0s workers (serial, drained), k0s controllers (serial), then `all:!proxmox:!dns:!power_only:!prd_k8s:!sandbox_k8s` | ops |
| `playbooks/ops/nfs_storage_check.yaml` | `prd_k8s:sandbox_k8s` (only clusters with NFS enabled) | ops |
| `playbooks/ops/shutdown.yaml` | `k8s_controller`, `k8s_worker`, `guest_shutdown_order`, `proxmox_shutdown_order` | ops (planned outage) |
| `playbooks/ops/startup.yaml` | `proxmox`, `prd_k8s_hypervisor`, `sandbox_k8s_hypervisor`, `guest_shutdown_order` (reversed), `k8s_controller` | ops (planned outage / per-cluster recovery) |
| `playbooks/ops/version_audit.yaml` | `lldap`, `authelia`, `forgejo`, `forgejo_runner`, `netbox`, `dnsdist`, `dns_resolver`, `dns_auth`, `seaweedfs`, `openbao`, `gpuvm`, `code_server`, `openclaw`, `hermes` | ops |
| `playbooks/ops/dns_failover_test.yaml` | delegated DNS hosts; orchestration on localhost | ops (state-changing failure test) |
| `playbooks/ops/lldap_cert_rotate.yaml` | `lldap`, then `sssd`, `authelia`, `blackbox_exporter` | ops (certificate rotation) |
| `playbooks/ops/pdns_sync.yaml` | `dns_primary`, `dns_secondary` | ops |
| `playbooks/ops/openbao/bootstrap.yaml` | `openbao` | ops |
| `playbooks/ops/openbao/configure.yaml` | `openbao` | ops |
| `playbooks/ops/openbao/configure_userpass.yaml` | `openbao` | ops |
| `playbooks/ops/openbao/register_cluster.yaml` | `openbao`, localhost kubeconfig/context | ops |
| `playbooks/ops/openbao/seed_secrets.yaml` | `openbao` | ops |
| `playbooks/ops/openbao/upgrade.yaml` | `openbao` | ops |

### DNS failover testing

`playbooks/ops/dns_failover_test.yaml` automates the ADR-0030 failure matrix. It stops
live DNS services, validates dnsdist health and DNS behavior, and restores all
stopped units in an `always` block after each scenario. An explicit disruption
confirmation is required:

```bash
ansible-playbook playbooks/ops/dns_failover_test.yaml \
  -e dns_failover_scenario=node2 \
  -e confirm_dns_disruption=true
```

AD is required when local DNSSEC validation is enabled. ADR-0051 trusted
forwarding allows responses without AD; bogus-name rejection remains required.
Both resolvers must use the same validation policy.

Supported scenarios are `resolver1`, `resolver2`, `node2`, `node3`,
`both_resolvers`, and `all`. If the controller is interrupted before the
`always` block completes, run the recovery action immediately:

```bash
ansible-playbook playbooks/ops/dns_failover_test.yaml \
  -e dns_failover_action=restore
```

## Configuration

Each role README documents its secret variables and inventory locations.
Keep secret values in SOPS-encrypted `group_vars` or `host_vars`; the vars plugin
loads them during execution.

DNS roles share backend lists in `inventories/homelab/group_vars/dns.yaml`.

### IP addresses: one source of truth

`inventories/homelab/hosts.yaml` is the only place a managed host's IP is a
literal. Everywhere else that needs to reach a host — `group_vars`,
`host_vars`, role defaults — derives it instead of repeating the literal:

- Referencing another host's address: `{{ hostvars['<name>']['ansible_host'] }}`
  (e.g. `dns.yaml`'s `dns_resolver_servers`, `proxmox.yaml`'s
  `proxmox_simplezone_routes_map.*.via`, `caddy.yaml`'s `caddy_upstreams`).
- Referencing the host's own address in its own `group_vars`/`host_vars`:
  `{{ ansible_host }}` (e.g. `dns_resolver.yaml`'s `knot_resolver_server_addr`,
  `caddy.yaml`'s `caddy_ip`).
- `hostvars[...]` works even for hosts not targeted by the current play,
  since `ansible_host` is an inventory variable, not a gathered fact — no
  `gather_facts` or play membership is required for the lookup to resolve.

Moving a host to a new IP then only means editing `hosts.yaml`; every
consumer re-resolves on the next run.

A literal IP is only correct when there is no corresponding inventory host to
point at instead — a device Ansible doesn't manage (e.g. `caddy.yaml`'s
`truenas-ui` backend), or an address that isn't a host at all, such as a
Kubernetes LoadBalancer IP (`all.yaml`'s `loki_endpoint` /
`alloy_otlp_server`) or a CIDR-based ACL (`dns_auth.yaml`'s
`pdns_webserver_allow_from`). Comment on any such literal explaining why it
can't be derived, so it doesn't get "fixed" into a broken `hostvars` lookup
later.
