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

A system playbook embeds only **service-coupled** concerns: the service role
plus the roles whose configuration the service *owns* — i.e. the log-shipping
stack (`vector` + `journald`, via the `lxc_logging` meta-role), since each
service supplies its own `vector_journald_units`. `timezone` is also embedded as a
pragmatic exception (cheap, idempotent, keeps log timestamps correct on a
single-playbook run). OpenClaw and Hermes also embed `mcp_client` to distribute
their service-owned caller credentials from SOPS.

It deliberately does **not** embed fleet-uniform **host hygiene** (`apt_mirror`,
`chrony`, `unattended_upgrades`, `node_exporter`): that config is identical on
every host, so duplicating it into each service playbook would violate DRY. The
test: *does this role's config vary per service?* If no, it belongs to the host
baseline, not the service playbook.

Every cross-cutting role additionally owns a `common/<role>.yaml` playbook so it
can be rolled out fleet-wide in one run.

### Downloading release artifacts

`get_url` defaults to a 10s socket timeout, which aborts mid-transfer on the
multi-MB release archives once egress leaves a SimpleZone. Every `get_url` task
therefore carries `retries: 3` / `delay: 5`, and artifact downloads (archives,
binaries, `.deb`) also raise `timeout` to 60s — 120s for the Caddy download API,
which builds the binary per request. Repository signing keys are a few KB and
keep the default timeout. `until` is omitted on purpose: with `retries` set and
no `until`, Ansible retries until the task stops failing.

### Provisioning a new host

Host hygiene is applied at host bring-up, before services, via `bootstrap.yaml`
(a thin aggregate that imports the baseline `common/` playbooks):

```bash
# 1. Baseline a freshly created host: package mirror, LXC maintenance access,
#    time, security updates, monitoring, logging, and k0s storage clients.
#    Per-play host patterns + --limit select the applicable subset automatically.
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
ansible-playbook playbooks/services/<system>.yaml --limit <host> --check --diff
ansible-playbook playbooks/services/<system>.yaml --limit <host>
```

Use the [Playbooks](#playbooks) index to choose an entry point. Service-specific
prerequisites and procedures live in the corresponding role README.

Operational examples:

```bash
# OS package upgrade (all hosts; apt on Debian/Ubuntu, dnf on Rocky/RHEL).
# DNS hosts and k0s nodes are upgraded one at a time; k0s workers are drained
# before the reboot and only uncordoned after the LocalPV mount is present. On
# NFS-enabled clusters, a temporary probe Pod is bound to that same worker and
# must mount the export successfully first. A full run therefore takes
# considerably longer than the parallel phase suggests.
ansible-playbook playbooks/ops/package_upgrade.yaml

# Same, one phase at a time (dns / k8s / others). The phases are independent,
# so a run that stopped partway can be resumed at the phase that failed.
ansible-playbook playbooks/ops/package_upgrade.yaml --tags others

# Planned outage: full shutdown, one phase at a time
# (workloads -> k8s -> guests -> hypervisors; run without --tags for all four)
ansible-playbook playbooks/ops/shutdown.yaml --tags workloads

# Planned outage: recovery after power is restored. This explicitly ensures all
# cluster guests are running, waits for dependencies, and restores the scaled-
# down workloads. Explicit starts are idempotent with tf's on_boot behavior.
# NFS-enabled clusters must pass a kubelet-backed mount probe before GitOps
# resumes and Argo CD restores workloads.
ansible-playbook playbooks/ops/startup.yaml

# Stop and restart one cluster while leaving its hypervisors running. The
# environment tag covers its workload, node-power, guest-start, readiness, and
# workload-restore phases; no --limit expression is needed.
ansible-playbook playbooks/ops/shutdown.yaml --tags sandbox
ansible-playbook playbooks/ops/startup.yaml --tags sandbox
ansible-playbook playbooks/ops/shutdown.yaml --tags prd
ansible-playbook playbooks/ops/startup.yaml --tags prd

# Version audit for Renovate-managed Ansible components (read-only)
ansible-playbook playbooks/ops/version_audit.yaml

# OpenBao explicit upgrade (dry-run first; the user runs without --check)
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
| `playbooks/services/lldap.yaml` | `lldap` | system |
| `playbooks/services/seaweedfs.yaml` | `seaweedfs` | system |
| `playbooks/services/openbao.yaml` | `openbao` | system |
| `playbooks/services/proxmox.yaml` | `proxmox` | system (platform) |
| `playbooks/services/gpuvm.yaml` | `gpuvm` | system |
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
| `playbooks/common/users.yaml` | `shared_vms` | cross-cutting |
| `playbooks/ops/package_upgrade.yaml` | `dns` (serial), k0s workers (serial, drained), k0s controllers (serial), then `all:!proxmox:!dns:!power_only:!prd_k8s:!sandbox_k8s` | ops |
| `playbooks/ops/nfs_storage_check.yaml` | `prd_k8s:sandbox_k8s` (only clusters with NFS enabled) | ops |
| `playbooks/ops/shutdown.yaml` | `k8s_controller`, `k8s_worker`, `guest_shutdown_order`, `proxmox_shutdown_order` | ops (planned outage) |
| `playbooks/ops/startup.yaml` | `proxmox`, `prd_k8s_hypervisor`, `sandbox_k8s_hypervisor`, `guest_shutdown_order` (reversed), `k8s_controller` | ops (planned outage / per-cluster recovery) |
| `playbooks/ops/version_audit.yaml` | `lldap`, `forgejo`, `forgejo_runner`, `netbox`, `dnsdist`, `dns_resolver`, `dns_auth`, `seaweedfs`, `openbao`, `gpuvm`, `code_server` | ops |
| `playbooks/ops/dns_failover_test.yaml` | delegated DNS hosts; orchestration on localhost | ops (state-changing failure test) |
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
