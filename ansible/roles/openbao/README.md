# openbao Role

Installs and configures [OpenBao](https://openbao.org/) secret management server on Debian-based systems.

## Functionality

- Installs OpenBao via the official apt repository.
- Deploys `/etc/openbao/openbao.hcl` from a Jinja2 template.
- Deploys the static seal key to `/etc/openbao/seal.key`.
- Applies a systemd drop-in to grant `CAP_IPC_LOCK` for mlock support.
- Ensures the service is started and enabled.

## Variables

### Secrets (must be set in SOPS-encrypted files)

| Variable | Description |
|----------|-------------|
| `openbao_seal_key` | Static seal key: 32 raw bytes, base64-encoded. Generate with `openssl rand -base64 32` |
| `openbao_root_token` | Root token from `operator init`. Stored as emergency backup; not used in day-to-day operations. |
| `openbao_admin_token` | Admin token created by `openbao_bootstrap.yaml`. Used by `openbao_configure.yaml`. |
| `openbao_k8s_token_reviewer_jwt` | JWT of the `openbao-token-reviewer` ServiceAccount in the prd cluster. Used to configure the `kubernetes/` auth method. |
| `openbao_k8s_ca_cert` | PEM-encoded CA certificate of the prd cluster. |
| `openbao_k8s_dev_token_reviewer_jwt` | JWT of the `openbao-token-reviewer` ServiceAccount in the dev cluster. Used to configure the `kubernetes-dev/` auth method. |
| `openbao_k8s_dev_ca_cert` | PEM-encoded CA certificate of the dev cluster. |
| `openbao_secrets` | List of KV secrets to write. Each entry requires `path` and `data` (key/value pairs). |
| `openbao_userpass_users` | List of userpass users. Each entry requires `username`, `password`, and `policies`. |
| `openbao_kubeconfigs` | List of kubeconfig files to seed. Each entry requires `name` (cluster name) and `path` (file path on the Ansible control node). |

### Non-secret variables (in `defaults/main.yaml`)

| Variable | Default | Description |
|----------|---------|-------------|
| `openbao_version` | `2.5.4` | OpenBao version to install |
| `openbao_user` | `openbao` | System user (created by package) |
| `openbao_group` | `openbao` | System group (created by package) |
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
| `openbao_k8s_host` | `""` | prd cluster API server URL (e.g. `https://192.168.30.11:6443`) |
| `openbao_k8s_dev_host` | `""` | dev cluster API server URL (e.g. `https://192.168.20.11:6443`) |
| `openbao_k8s_clusters` | see defaults | List of Kubernetes clusters to configure auth for. Each entry defines `mount_path`, `host`, `ca_cert`, `ca_cert_file`, `token_reviewer_jwt`, `role`, and `policies`. |

## Post-install initialization

The role starts the OpenBao service but does **not** initialize it. On first deploy, run the following on the server:

```bash
BAO_ADDR=http://127.0.0.1:8200 bao operator init
```

Save the output (root token and recovery keys) securely. After completing initial configuration, revoke the root token:

```bash
bao token revoke <root_token>
```

## Kubernetes ESO integration setup

After initialization, run the following playbooks in order to set up OpenBao for use with External Secrets Operator.

OpenBao manages two Kubernetes clusters:

| Auth mount | Cluster | ESO ArgoCD app |
|---|---|---|
| `kubernetes/` | prd (`192.168.30.11`) | `k8s/argocd/prd/apps/eso.yaml` |
| `kubernetes-dev/` | dev (`192.168.20.11`) | `k8s/argocd/dev/apps/eso.yaml` (overrides `mountPath`) |

### 1. Bootstrap admin token (run once)

Add the root token to `group_vars/openbao.sops.yaml`:

```yaml
openbao_root_token: "hvs.xxxx"
openbao_admin_token: ""  # filled in next step
```

Run the bootstrap playbook:

```bash
ansible-playbook playbooks/openbao_bootstrap.yaml
```

Copy the `openbao_admin_token` value from the output into `group_vars/openbao.sops.yaml`.

### 2. Apply Kubernetes token reviewer manifest (both clusters)

Run on each cluster to create the `openbao-token-reviewer` ServiceAccount:

```bash
kubectl create namespace external-secrets --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f k8s/eso/templates/token-reviewer.yaml
```

Retrieve the JWT and CA cert from each cluster:

```bash
# JWT
kubectl get secret openbao-token-reviewer -n external-secrets \
  -o jsonpath='{.data.token}' | base64 -d

# CA cert
kubectl get secret openbao-token-reviewer -n external-secrets \
  -o jsonpath='{.data.ca\.crt}' | base64 -d
```

Add all values to `group_vars/openbao.sops.yaml`:

```yaml
openbao_k8s_token_reviewer_jwt: "eyJ..."   # prd cluster
openbao_k8s_ca_cert: |
  -----BEGIN CERTIFICATE-----
  ...
  -----END CERTIFICATE-----
openbao_k8s_dev_token_reviewer_jwt: "eyJ..."   # dev cluster
openbao_k8s_dev_ca_cert: |
  -----BEGIN CERTIFICATE-----
  ...
  -----END CERTIFICATE-----
```

### 3. Configure OpenBao

```bash
ansible-playbook playbooks/openbao_configure.yaml
```

This enables KV v2, configures Kubernetes auth for both clusters, and creates policies and roles.

### 4. Install ESO via ArgoCD

Push the changes to git. ArgoCD will sync both ESO apps and install ESO on each cluster.
Verify the `ClusterSecretStore` becomes `Ready` on each cluster:

```bash
kubectl get clustersecretstore openbao
```

## KV Path Conventions

All secrets are stored under the `secret/` KV v2 mount. Two top-level namespaces are used:

### `secret/k8s/{app}/{secret}`

Secrets consumed by Kubernetes applications via ESO. Scoped per application; not shared across consumers.

```
secret/k8s/cert-manager/cloudflare   # Cloudflare API token
secret/k8s/headlamp/admin-token      # Headlamp login token
secret/k8s/monitoring/grafana        # Grafana credentials
```

### `secret/kubeconfig/{cluster}`

Kubeconfig files for accessing Kubernetes clusters. Shared across multiple consumers (ESO for Headlamp, VM provisioning via `bao` CLI, etc.).

```
secret/kubeconfig/dev   # dev cluster kubeconfig
secret/kubeconfig/prd   # prd cluster kubeconfig
```

The distinction: `k8s/` is for secrets **used by** apps running in Kubernetes; `kubeconfig/` is for credentials **to access** Kubernetes clusters.

## Seeding Kubeconfigs

Configure `openbao_kubeconfigs` in `group_vars/openbao.sops.yaml`:

```yaml
openbao_kubeconfigs:
  - name: dev
    path: ~/.kube/dev.yaml
  - name: prd
    path: ~/.kube/prd.yaml
```

Run the dedicated playbook:

```bash
ansible-playbook playbooks/openbao_seed_kubeconfig.yaml
```

The playbook runs `bao kv put` from the Ansible control node directly against the OpenBao API (`openbao_api_addr`). No files are copied to the OpenBao host.

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
ansible-playbook playbooks/openbao_configure_userpass.yaml
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
