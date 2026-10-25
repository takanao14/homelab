# openbao Role

Installs and configures [OpenBao](https://openbao.org/) secret management server on Debian-based systems.

## Functionality

- Installs OpenBao via the official apt repository.
- Holds the `openbao` apt package by default so fleet package upgrades do not
  advance the stateful secret store ahead of the reviewed version.
- Deploys `/etc/openbao/openbao.hcl` from a Jinja2 template.
- Deploys the static seal key to `/etc/openbao/seal.key`.
- Applies a systemd drop-in to grant `CAP_IPC_LOCK` for mlock support.

## Variables

### Secrets (must be set in SOPS-encrypted files)

| Variable | Description |
|----------|-------------|
| `openbao_seal_key` | Static seal key: 32 raw bytes, base64-encoded. Generate with `openssl rand -base64 32` |
| `openbao_root_token` | Root token from `operator init`. Used only for bootstrap and recovery. |
| `openbao_ansible_password` | Password for the dedicated Ansible userpass account. Generate with `openssl rand -base64 24`. |
| `openbao_secrets` | List of KV secrets to write. Each entry requires `path` and `data` (key/value pairs). |
| `openbao_argocd_admin` | ArgoCD admin password per environment. Each entry requires `env` and `password` (bcrypt hash). Written on change only, with `mtime` stamped automatically — see [ArgoCD admin password](#argocd-admin-password). |
| `openbao_userpass_users` | List of userpass users. Each entry requires `username`, `password`, and `policies`. |

### Non-secret variables (in `defaults/main.yaml`)

| Variable | Default | Description |
|----------|---------|-------------|
| `openbao_version` | [defaults/main.yaml](defaults/main.yaml) | Reviewed OpenBao version used by Renovate, audit, fresh installs, and the explicit upgrade workflow |
| `openbao_user` | `openbao` | System user (created by package) |
| `openbao_group` | `openbao` | System group (created by package) |
| `openbao_package_hold` | `true` | Hold the `openbao` apt package during normal service-role runs; the explicit upgrade workflow manages this hold |
| `openbao_backup_dir` | `/var/backups/openbao` | Root-only directory for pre-upgrade Raft snapshots |
| `openbao_config_dir` | `/etc/openbao` | Config directory |
| `openbao_data_dir` | `/opt/openbao/data` | Raft storage directory |
| `openbao_binary` | `/usr/bin/bao` | Binary path |
| `openbao_seal_key_path` | `/etc/openbao/seal.key` | Seal key file path |
| `openbao_api_port` | `8200` | API listen port |
| `openbao_cluster_port` | `8201` | Raft cluster port |
| `openbao_api_addr` | `https://<host_ip>:8200` | Public API address (override to Caddy URL) |
| `openbao_cluster_addr` | `http(s)://<host_ip>:8201` | Raft cluster address (protocol follows `openbao_tls_disable`) |
| `openbao_tls_disable` | `true` | Disable TLS on the listener (use when fronted by a reverse proxy) |
| `openbao_tls_cert_file` | `/etc/openbao/tls/cert.pem` | TLS certificate path (used when `openbao_tls_disable: false`) |
| `openbao_tls_key_file` | `/etc/openbao/tls/key.pem` | TLS key path (used when `openbao_tls_disable: false`) |
| `openbao_raft_retry_join` | `[]` | List of other cluster node API addresses for Raft auto-join |
| `openbao_seal_key_id` | `key-1` | Permanent identifier for the static seal key (update when rotating) |
| `openbao_local_addr` | `http://127.0.0.1:8200` | API address used by `bao` CLI tasks running on the openbao host. Plain HTTP because TLS is terminated by Caddy upstream. |
| `openbao_ansible_user` | `ansible-admin` | Dedicated userpass account for OpenBao operational playbooks |
| `openbao_k8s_host` | `""` | prd cluster API server URL (e.g. `https://192.168.60.11:6443`) |
| `openbao_k8s_sandbox_host` | `""` | sandbox cluster API server URL (e.g. `https://192.168.20.31:6443`) |
| `openbao_k8s_clusters` | see defaults | List of Kubernetes clusters to configure auth for. Each entry defines `name`, `mount_path`, `host`, `ca_cert_file`, `role`, and `policies`. Runtime CA data is injected by `playbooks/ops/openbao/register_cluster.yaml`. |

## Upgrade

`openbao_version` is the reviewed upstream release. Renovate updates it, but the
normal service role installs it only on a fresh host and otherwise keeps the apt
package held. Upgrade OpenBao only through the operational playbook:

```bash
# Read-only desired-versus-installed check
ansible-playbook playbooks/ops/version_audit.yaml --limit openbao

# Validate the upgrade without changing the host
ansible-playbook playbooks/ops/openbao/upgrade.yaml --limit openbao --check --diff

# The operator performs the state-changing upgrade
ansible-playbook playbooks/ops/openbao/upgrade.yaml --limit openbao

# Verify the resulting package version
ansible-playbook playbooks/ops/version_audit.yaml --limit openbao
```

The upgrade playbook runs one OpenBao host at a time, requires an initialized
and unsealed server, confirms that the reviewed release exists in the apt
repository, prevents downgrades, and saves a Raft snapshot before changing the
package. It restores the apt hold even when the package task fails, then checks
the local API and installed package version. Snapshot files remain under
`openbao_backup_dir`; remove them manually according to the backup retention
policy.

## Post-install initialization

The role starts the OpenBao service but does **not** initialize it. On first deploy, run
from the workstation:

```bash
SOPS_AGE_KEY_FILE="$HOME/.config/sops/age/keys.txt" scripts/openbao-init.py <user>@192.168.40.30
```

It refuses an initialized server, runs `operator init` over SSH, and writes the root
token and recovery keys into `openbao_root_token` and `openbao_recovery_keys` of the
SOPS-encrypted inventory without printing them. Neither is used by routine
operational playbooks. The recovery keys authorize `operator generate-root` and
`operator rekey`; they do not unseal, because the static seal does that. Keep
every share — rotating them requires the existing ones, so a lost set cannot be
regenerated.

## Rebuilding the host

Recover OpenBao by rebuilding it from its declarations rather than restoring
a snapshot (ADR-0042).
KV entries come from SOPS inventory and the workstation admin scripts below.
Run the steps in order:

1. Re-create the VM, then `ansible-playbook playbooks/services/openbao.yaml`. The role
   installs the reviewed `openbao_version`, deploys `openbao.hcl` and the static seal
   key from SOPS, so the rebuilt host unseals on its own once initialized.
2. Run `scripts/openbao-init.py` as in [Post-install initialization](#post-install-initialization).
3. **Commit the updated SOPS file before continuing.** The new `operator init` issues
   a new root token and new recovery keys, so the previous values are dead. Bootstrap
   authenticates with the stored root token and fails until they are replaced.
4. `playbooks/ops/openbao/bootstrap.yaml`, then `playbooks/ops/openbao/configure.yaml` for the KV
   mount and policies, then `playbooks/ops/openbao/configure_userpass.yaml`, then
   `playbooks/ops/openbao/seed_secrets.yaml`.
5. Re-inject the hand-managed entries with `scripts/secrets/admin/`:
   `set-sops-key.sh`, `set-env.sh`, `set-kubeconfig.sh`. These log in as the
   `admin` userpass account created in the previous step.
6. `playbooks/ops/openbao/register_cluster.yaml -e cluster=prd` and again for `sandbox`.
   Each run creates and configures that cluster's Kubernetes auth mount before
   writing its ESO role.
7. Run `scripts/secrets/admin/seed-mcp.sh` for the prd MCP secrets.

## Reconciling declared and stored secrets

Out-of-band writes accumulate, so compare the declaration against the live store
periodically with `scripts/check-openbao-secrets.sh` (see
[`scripts/README.md`](../../../scripts/README.md)). It reports orphans, which are
destroyed with `bao kv metadata delete`, and paths left in `openbao_secrets`
after the secret was retired, which the next seed run recreates on the server.

## Kubernetes ESO integration setup

After initialization, run the following playbooks in order to set up OpenBao for use with External Secrets Operator.

OpenBao manages two Kubernetes clusters:

| Auth mount | Cluster | ESO mountPath values |
|---|---|---|
| `kubernetes/` | prd (`192.168.60.11`) | `k8s/eso/prd/values.yaml` |
| `kubernetes-sandbox/` | sandbox (`192.168.20.31`) | `k8s/eso/sandbox/values.yaml` |

The app-of-apps chart (`k8s/argocd/apps`) renders the ESO ArgoCD Application.
Enable it per environment in `k8s/argocd/<env>/apps-values.yaml`.

Application policies grant read-only access to their own KV prefix. The sandbox
ESO role has only `k8s-sandbox` (`secret/k8s-sandbox/*`) and
`k8s-argocd-sandbox`: sandbox never reads prd application credentials
(ADR-0070).
The prd role also includes `k8s-mcp`; `scripts/secrets/admin/seed-mcp.sh`
idempotently seeds its two paths from `.env/secrets.sops.env` after policy
configuration. Re-run it after rotating either upstream or caller token.
Apply `playbooks/ops/openbao/configure.yaml`, then
`playbooks/ops/openbao/register_cluster.yaml -e cluster=prd` to attach the new policy to
the live ESO role before seeding or syncing MCP.

List retired policies in `openbao_absent_policies` to remove their server-side
objects as well.
`playbooks/ops/openbao/configure.yaml` deletes those server-side policy objects before it
writes the active policies and Kubernetes roles.

### 1. Bootstrap Ansible userpass authentication

Add the root token and a newly generated Ansible password to
`group_vars/openbao.sops.yaml`:

```yaml
openbao_root_token: "hvs.xxxx"
openbao_ansible_password: "<output of openssl rand -base64 24>"
```

Run the bootstrap playbook:

```bash
ansible-playbook playbooks/ops/openbao/bootstrap.yaml
```

The playbook enables userpass and creates or updates `ansible-admin` with the
`admin` policy and a one-hour token TTL. Operational playbooks log in at runtime
with this account, retain the temporary token only for the current play, and do
not require a stored admin token. Re-run bootstrap after changing
`openbao_ansible_password` to rotate the account password.

### 2. Configure OpenBao base objects

```bash
ansible-playbook playbooks/ops/openbao/configure.yaml
```

This enables KV v2 and configures policies.
`playbooks/ops/openbao/register_cluster.yaml` refreshes each cluster's Kubernetes
auth mount, auth config, and role so cluster CA rotation does not require
editing SOPS secrets.

### 3. Install ESO via ArgoCD

Push the changes to git. Argo CD installs ESO on each cluster. Wait for the
controller before registration:

```bash
kubectl -n external-secrets rollout status deploy/external-secrets
```

`ClusterSecretStore/openbao` can remain unready until the next step configures
Kubernetes authentication.

### 4. Register each cluster with OpenBao

Run the registration playbook after Argo CD reconciles ESO. It reads
the target cluster CA from the local kubeconfig, writes the OpenBao Kubernetes
auth config with `disable_local_ca_jwt=true`, restarts ESO, and validates
`ExternalSecret` readiness.

```bash
ansible-playbook playbooks/ops/openbao/register_cluster.yaml -e cluster=prd
ansible-playbook playbooks/ops/openbao/register_cluster.yaml -e cluster=sandbox
```

By default the playbook reads `~/.kube/<cluster>.yaml` and uses the
`<cluster>-homelab` kube context. Override either value when needed:

```bash
ansible-playbook playbooks/ops/openbao/register_cluster.yaml \
  -e cluster=sandbox \
  -e kubeconfig=/path/to/kubeconfig

ansible-playbook playbooks/ops/openbao/register_cluster.yaml \
  -e cluster=sandbox \
  -e kube_context=sandbox-homelab
```

After registration, verify `kubectl get clustersecretstore openbao` reports
`Ready`. Run the same registration command after rebuilding a k0s cluster.
OpenBao KV secret values remain intact because OpenBao is external to the Kubernetes cluster.

## KV Path Conventions

Store all KV secrets under the `secret/` KV v2 mount. Application and
kubeconfig paths follow the conventions below; for workstation `.env` and AGE key
paths, see [scripts/README.md](../../../scripts/README.md#secrets--environment).

### `secret/k8s/{app}/{secret}`

Secrets consumed by Kubernetes applications via ESO. Policies scope access by
application; pdns-ui also reads the external-dns PowerDNS key.

```
secret/k8s/cert-manager/cloudflare   # Cloudflare API token
secret/k8s/monitoring/grafana        # Grafana credentials
secret/k8s-sandbox/grafana           # sandbox Grafana credentials
secret/k8s/monitoring/alertmanager   # Alertmanager Discord webhook
secret/k8s/argocd/{env}/admin        # ArgoCD admin password (per environment)
```

`k8s/argocd/` is the one app that splits by environment, because prd and
sandbox have independent ArgoCD admin passwords.
`tasks/configure_policies.yaml` generates matching `k8s-argocd-{env}` policies
per cluster so neither cluster's ESO can read the other's hash.

### `secret/kubeconfig/{cluster}`

Kubeconfig files for accessing Kubernetes clusters, retrieved from a workstation
with `scripts/secrets/get-kubeconfig.sh` and by VM provisioning via the `bao`
CLI. No ESO policy grants access to this prefix: in-cluster workloads use their
own ServiceAccount, so a cluster never reads a kubeconfig out of OpenBao.

```
secret/kubeconfig/prd   # prd cluster kubeconfig
secret/kubeconfig/sandbox # sandbox cluster kubeconfig
```

The distinction: `k8s/` is for secrets **used by** apps running in Kubernetes; `kubeconfig/` is for credentials **to access** Kubernetes clusters.

Seed application secrets from the SOPS-encrypted `openbao_secrets` list,
except the Argo CD entries below and MCP tokens, which
`scripts/secrets/admin/seed-mcp.sh` seeds.

### ArgoCD admin password

This one lives in its own variable, `openbao_argocd_admin`, not in
`openbao_secrets`. Store only the bcrypt hash — **never the plaintext**:

```yaml
openbao_argocd_admin:
  - env: prd
    password: "$2a$10$..."
  - env: sandbox
    password: "$2a$10$..."
```

Generate the hash with:

```bash
htpasswd -nbBC 10 "" '<password>' | tr -d ':\n' | sed 's/$2y/$2a/'
```

The `$` characters need no escaping — the seeding tasks use
`ansible.builtin.command`, which does not invoke a shell. Quote the value in
YAML so it is not mistaken for an alias or tag.

`env` must match an `openbao_k8s_clusters[].name`, since that is what generates
the `k8s-argocd-<env>` read policy. The seeding task writes the entry to
`secret/k8s/argocd/<env>/admin`.

`tasks/seed_argocd_admin.yaml` writes the hash and a UTC `mtime` only when the
hash changes, invalidating admin sessions only on rotation. Keep these entries
outside `openbao_secrets`, whose unconditional writes would reset `mtime` and
invalidate sessions on every seed run.

See [`k8s/argocd/README.md`](../../../k8s/argocd/README.md) for the consuming
`ExternalSecret`.

Then run:

```bash
ansible-playbook playbooks/ops/openbao/seed_secrets.yaml
```

## Userpass auth

To add human operators or application accounts, add entries to `openbao_userpass_users` in `group_vars/openbao.sops.yaml`:

```yaml
openbao_userpass_users:
  - username: alice
    password: "xxxx"
    policies: "kv-admin"
  - username: ci-reader
    password: "xxxx"
    policies: "kv-read"
```

Available policies:

| Policy | Capabilities |
|--------|-------------|
| `kv-admin` | create, read, update, delete, list on `secret/*` |
| `kv-read` | read, list on `secret/data/*` and `secret/metadata/*` |

Run the playbook to apply:

```bash
ansible-playbook playbooks/ops/openbao/configure_userpass.yaml
```

Login with the bao CLI:

```bash
bao login -method=userpass username=alice
```

## Expanding to a 3-node Raft cluster

Set `openbao_raft_retry_join` in `group_vars/openbao.yaml` and re-run the playbook:

```yaml
openbao_raft_retry_join:
  - "http://192.168.40.31:8200"
  - "http://192.168.40.32:8200"
```

## Usage

Run [playbooks/services/openbao.yaml](../../playbooks/services/openbao.yaml).
