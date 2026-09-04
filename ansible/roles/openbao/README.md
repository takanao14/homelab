# openbao Role

Installs and configures [OpenBao](https://openbao.org/) secret management server on Debian-based systems.

## Functionality

- Installs OpenBao via the official apt repository.
- Holds the `openbao` apt package by default so fleet package upgrades do not
  advance the stateful secret store ahead of the reviewed version.
- Deploys `/etc/openbao/openbao.hcl` from a Jinja2 template.
- Deploys the static seal key to `/etc/openbao/seal.key`.
- Applies a systemd drop-in to grant `CAP_IPC_LOCK` for mlock support.
- Ensures the service is started and enabled.

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
| `openbao_version` | `2.6.1` | Reviewed OpenBao version used by Renovate, audit, and the explicit upgrade workflow |
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
| `openbao_k8s_clusters` | see defaults | List of Kubernetes clusters to configure auth for. Each entry defines `name`, `mount_path`, `host`, `ca_cert_file`, `role`, and `policies`. Runtime CA data is injected by `ops-openbao_register_cluster.yaml`. |

## Upgrade

`openbao_version` is the reviewed upstream release. Renovate updates it, but the
normal service role deliberately keeps the apt package held. Upgrade OpenBao
only through the operational playbook:

```bash
# Read-only desired-versus-installed check
ansible-playbook playbooks/ops-version_audit.yaml --limit openbao

# Validate the upgrade without changing the host
ansible-playbook playbooks/ops-openbao_upgrade.yaml --limit openbao --check --diff

# The operator performs the state-changing upgrade
ansible-playbook playbooks/ops-openbao_upgrade.yaml --limit openbao

# Verify the resulting package version
ansible-playbook playbooks/ops-version_audit.yaml --limit openbao
```

The upgrade playbook runs one OpenBao host at a time, requires an initialized
and unsealed server, confirms that the reviewed release exists in the apt
repository, prevents downgrades, and saves a Raft snapshot before changing the
package. It restores the apt hold even when the package task fails, then checks
the local API and installed package version. Snapshot files remain under
`openbao_backup_dir`; remove them manually according to the backup retention
policy.

## Post-install initialization

The role starts the OpenBao service but does **not** initialize it. On first deploy, run the following on the server:

```bash
BAO_ADDR=http://127.0.0.1:8200 bao operator init
```

Save the recovery keys securely. Store the root token only in the
SOPS-encrypted inventory; it is required by the bootstrap and recovery path but
is not used by routine operational playbooks.

## Kubernetes ESO integration setup

After initialization, run the following playbooks in order to set up OpenBao for use with External Secrets Operator.

OpenBao manages two Kubernetes clusters:

| Auth mount | Cluster | ESO mountPath values |
|---|---|---|
| `kubernetes/` | prd (`192.168.60.11`) | `k8s/eso/prd/values.yaml` |
| `kubernetes-sandbox/` | sandbox (`192.168.20.31`) | `k8s/eso/sandbox/values.yaml` |

The ESO ArgoCD Application itself is rendered by the app-of-apps chart
(`k8s/argocd/apps`) and enabled per environment in
`k8s/argocd/<env>/apps-values.yaml`.

Application policies grant read-only access to their own KV prefix. The prd ESO
role includes `k8s-cert-manager`, while sandbox deliberately does not because
its Argo CD bootstrap runs HTTP-only without cert-manager (ADR-0010).

Policies removed from the repository are listed in `openbao_absent_policies`.
`ops-openbao_configure.yaml` deletes those server-side policy objects before it
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
ansible-playbook playbooks/ops-openbao_bootstrap.yaml
```

The playbook enables userpass and creates or updates `ansible-admin` with the
`admin` policy and a one-hour token TTL. Operational playbooks log in at runtime
with this account, retain the temporary token only for the current play, and do
not require a stored admin token. Re-run bootstrap after changing
`openbao_ansible_password` to rotate the account password.

### 2. Configure OpenBao base objects

```bash
ansible-playbook playbooks/ops-openbao_configure.yaml
```

This enables KV v2 and configures policies. Kubernetes auth mounts, auth config,
and roles are refreshed per cluster by `ops-openbao_register_cluster.yaml` so
cluster CA rotation does not require editing SOPS secrets.

### 3. Install ESO via ArgoCD

Push the changes to git. ArgoCD will sync the ESO apps and install ESO on each cluster.
Verify the `ClusterSecretStore` becomes `Ready` on each cluster:

```bash
kubectl get clustersecretstore openbao
```

### 4. Register each cluster with OpenBao

Run the registration playbook after ESO has been reconciled by ArgoCD. It reads
the target cluster CA from the local kubeconfig, writes the OpenBao Kubernetes
auth config with `disable_local_ca_jwt=true`, restarts ESO, and validates
`ExternalSecret` readiness.

```bash
ansible-playbook playbooks/ops-openbao_register_cluster.yaml -e cluster=prd
ansible-playbook playbooks/ops-openbao_register_cluster.yaml -e cluster=sandbox
```

By default the playbook reads `~/.kube/<cluster>.yaml` and uses the
`<cluster>-homelab` kube context. Override either value when needed:

```bash
ansible-playbook playbooks/ops-openbao_register_cluster.yaml \
  -e cluster=sandbox \
  -e kubeconfig=/path/to/kubeconfig

ansible-playbook playbooks/ops-openbao_register_cluster.yaml \
  -e cluster=sandbox \
  -e kube_context=sandbox-homelab
```

Run the same command after rebuilding a k0s cluster. OpenBao KV secret values
remain intact because OpenBao is external to the Kubernetes cluster.

## KV Path Conventions

All secrets are stored under the `secret/` KV v2 mount. Two top-level namespaces are used:

### `secret/k8s/{app}/{secret}`

Secrets consumed by Kubernetes applications via ESO. Scoped per application; not shared across consumers.

```
secret/k8s/cert-manager/cloudflare   # Cloudflare API token
secret/k8s/monitoring/grafana        # Grafana credentials
secret/k8s/monitoring/alertmanager   # Alertmanager Discord webhook
secret/k8s/argocd/{env}/admin        # ArgoCD admin password (per environment)
```

`k8s/argocd/` is the one app that splits by environment, because prd and
sandbox have independent ArgoCD admin passwords. The matching
`k8s-argocd-{env}` policies are generated per cluster in
`tasks/configure_policies.yaml`, so neither cluster's ESO can read the other's
hash.

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

Secrets are seeded from the SOPS-encrypted `openbao_secrets` list.

For the Alertmanager Discord receiver, add:

```yaml
- path: secret/k8s/monitoring/alertmanager
  data:
    discord-webhook-url: "<Discord webhook URL>"
```

### ArgoCD admin password

This one lives in its own variable, `openbao_argocd_admin`, not in
`openbao_secrets`. Only the bcrypt hash is stored by hand — **never the
plaintext**:

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
the `k8s-argocd-<env>` read policy. The entry is written to
`secret/k8s/argocd/<env>/admin`.

`tasks/seed_argocd_admin.yaml` adds the second property, `mtime`, on its own:
it reads the hash currently in OpenBao, and writes the entry — stamping `mtime`
with the current UTC time — **only when the hash differs**. Rotating a password
therefore invalidates every ArgoCD admin session for that environment, while
re-running the playbook for any other reason changes nothing.

That is why it is not part of `openbao_secrets`: the generic loop rewrites its
whole list unconditionally, so a shared `mtime` would be re-stamped on every
seed run and log all admins out each time.

See [`k8s/argocd/README.md`](../../../k8s/argocd/README.md) for the consuming
`ExternalSecret`.

Then run:

```bash
ansible-playbook playbooks/ops-openbao_seed_secrets.yaml
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
ansible-playbook playbooks/ops-openbao_configure_userpass.yaml
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

## Dependencies

None.

## Usage

```yaml
- name: Setup OpenBao
  hosts: openbao
  roles:
    - role: openbao
```
