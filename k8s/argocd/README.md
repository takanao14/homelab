# ArgoCD

Argo CD GitOps configuration for `prd` and `sandbox`.

## Directory Structure

```
argocd/
├── values-common.yaml        # Shared upstream and local-chart values
├── chart/                    # HTTPRoute and admin ExternalSecret
│   └── templates/
├── apps/                     # App-of-apps chart
│   ├── Chart.yaml
│   ├── values.yaml           # Defaults, waves, and upstream versions
│   └── templates/
├── prd/
│   ├── helmfile.yaml         # Bootstrap only
│   ├── values.yaml           # Environment values
│   ├── apps-values.yaml      # Enabled apps
│   └── root-apps.yaml        # Root Application
└── sandbox/
    └── ...                   # Same layout as prd
```

## Environments

| Environment | Cluster | ArgoCD URL |
|-------------|---------|------------|
| prd | prd-homelab | `argocd.prd.butaco.net` |
| sandbox | sandbox-homelab | `http://argocd.sandbox.butaco.net` |

> `butaco.net` is a personal domain. Replace with your own domain in `prd/values.yaml` and `sandbox/values.yaml`.

## Initial Deployment

Helmfile bootstraps Argo CD; App of Apps then takes ownership. Do not rerun
`helmfile apply` after applying `root-apps.yaml`.

```bash
# prd environment
cd k8s/argocd/prd
helmfile apply

# Apply root App of Apps
kubectl apply -f k8s/argocd/prd/root-apps.yaml
```

Hooks reject the wrong context and self-managed releases.
`ARGOCD_BOOTSTRAP_FORCE=1` permits deliberate re-bootstrap.

For sandbox, use `k8s/argocd/sandbox`. It intentionally exposes ArgoCD over
HTTP only and does not install cert-manager.

### Getting in on a fresh cluster

Without `admin.password`, the server creates `argocd-initial-admin-secret`:

```bash
kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d
```

Keep it until ESO syncs after OpenBao cluster registration
([ADR-0012](../../docs/adr/0012-openbao-eso-cluster-rebuild-registration.md)):

1. Re-register the cluster
   (`ops-openbao_register_cluster.yaml -e cluster=<env>`).
2. Wait for `kubectl -n argocd get externalsecret argocd-admin-password` to
   report `SecretSynced`.
3. Log in with the password from OpenBao.
4. Only then `kubectl -n argocd delete secret argocd-initial-admin-secret`.

For recovery, remove `admin.password`; the server regenerates the bootstrap
secret until ESO overwrites it:

```bash
kubectl -n argocd patch secret argocd-secret --type=json -p '[{"op":"remove","path":"/data/admin.password"}]'
```

### Changing values

After bootstrap, commit and push value changes; `selfHeal` applies them.
`helmfile apply` fails because SSA-managed resources lack Helm ownership:

```
invalid ownership metadata; annotation validation error:
missing key "meta.helm.sh/release-name"
```

The bootstrap Helm release may remain behind the Git-managed chart version;
this is expected.

## Secrets Management

ESO supplies application secrets from OpenBao; see `k8s/eso/`.

### Admin password

ESO merges each environment's OpenBao-backed `admin.password` and
`admin.passwordMtime` into `argocd-secret`:

| Environment | OpenBao key |
|-------------|-------------|
| prd | `k8s/argocd/prd/admin` |
| sandbox | `k8s/argocd/sandbox/admin` |

Each key holds:

| Property | Value | Maintained by |
|----------|-------|---------------|
| `password` | bcrypt hash of the password — **never the plaintext** | operator |
| `mtime` | RFC3339 timestamp | Ansible, automatically |

Store only the hash in the SOPS-encrypted `openbao_argocd_admin` list.
`ops-openbao_seed_secrets.yaml` writes it and stamps UTC `mtime`. See
[`ansible/roles/openbao/README.md`](../../ansible/roles/openbao/README.md) for
the entry format and the hash command.

Per-cluster `k8s-argocd-{env}` policies isolate access. The server rejects
sessions older than `mtime` without restarting; invalid or missing `mtime`
disables the cutoff.

Why this shape rather than `configs.secret.argocdServerAdminPassword`:

- It exposes the bcrypt hash in Git, prohibited by ADR-0026.
- Its default `now()` mtime creates permanent drift and repeated logout.

`creationPolicy: Merge` preserves `server.secretkey`. Password sync does not
block waves; failed ExternalSecrets retry.

#### Rollout

1. Add each environment and bcrypt hash to `openbao_argocd_admin` in
   `openbao.sops.yaml`.
2. Seed them and grant read access:

   ```bash
   ansible-playbook ansible/playbooks/ops-openbao_seed_secrets.yaml
   ansible-playbook ansible/playbooks/ops-openbao_configure.yaml
   ```

   Re-register each cluster so the ESO role receives the new policy:

   ```bash
   ansible-playbook ansible/playbooks/ops-openbao_register_cluster.yaml -e cluster=prd
   ansible-playbook ansible/playbooks/ops-openbao_register_cluster.yaml -e cluster=sandbox
   ```

3. Commit and push; the `argocd` Application syncs automatically.
4. Confirm the ExternalSecret resolved:
   `kubectl -n argocd get externalsecret argocd-admin-password`
5. Log in, then delete `argocd-initial-admin-secret`.

Complete OpenBao steps before pushing; otherwise the ExternalSecret waits in
`SecretSyncedError` without blocking Argo CD.

#### Rotating the password

Replace the hash and rerun `ops-openbao_seed_secrets.yaml`.

**Hourly ESO refresh.** Force immediate synchronization with:

```bash
kubectl -n argocd annotate externalsecret argocd-admin-password force-sync=$(date +%s) --overwrite
```

Confirm `admin.passwordMtime` matches the seeding run:

```bash
kubectl -n argocd get secret argocd-secret -o jsonpath='{.data.admin\.passwordMtime}' | base64 -d
```

Five failures lock the account for five minutes. Further attempts extend it;
stop and inspect logs:

```bash
kubectl -n argocd logs deploy/argocd-server --since=10m | grep -E "failed login|too many failed"
```

| Log line | Meaning |
|----------|---------|
| `User admin failed login N time(s)` | password really was compared and did not match |
| `User admin had too many failed logins (5)` | locked out; the password was never checked |

No Redis login key means the lockout expired:

```bash
P=$(kubectl -n argocd get secret argocd-redis -o jsonpath='{.data.auth}' | base64 -d); kubectl -n argocd exec deploy/argocd-redis -- sh -c "REDISCLI_AUTH='$P' redis-cli --scan --pattern 'login*'"
```

If the password is wrong, verify locally before reseeding. Single-quote it to
prevent zsh history expansion:

```bash
printf 'admin:%s\n' '<hash>' > /tmp/h && htpasswd -bv /tmp/h admin '<password>'; rm -f /tmp/h
```

## HTTPRoute

The shared chart renders an HTTPRoute from each environment's
`server.ingress.hostname`.

The Application combines:

1. Upstream `argo-cd` Helm chart
2. Values ref (this repo)
3. `k8s/argocd/chart` — renders HTTPRoute from values

## App of Apps

Each root renders enabled Applications from the shared `apps/` chart and its
environment values (ADR-0014).

To deploy an app to another environment:

1. Set `apps.<name>.enabled: true` in `k8s/argocd/<env>/apps-values.yaml`.
2. Add `k8s/<app>/<env>/values.yaml` if the app takes per-env values.

For a new app, add its template and disabled default/wave. Keep upstream chart
coordinates in `apps/values.yaml` where Renovate can detect them.

Inspect the rendered Applications with:

```bash
helm template k8s/argocd/apps -f k8s/argocd/<env>/apps-values.yaml
```

### Sync waves

Waves are defined once in `apps/values.yaml` and shared by all environments:

| Wave | Applications | Rationale |
|------|--------------|-----------|
| -2 | envoy-gateway-crds | Gateway API CRDs (sole owner) |
| -1 | envoy-gateway | Controller after its CRDs |
| 0 | cert-manager, eso, gateway | Foundation: CRDs, ClusterSecretStore, shared Gateway |
| 1 | everything else | Consumers of wave 0 (ExternalSecrets, HTTPRoutes, issuers) |

The custom Application health check gates waves. Explicit retries cover
bootstrap degradation and CRD registration races.

Caveat: the wave-0 HTTPS listener references wave-1 certificate resources. If a
fresh prd Gateway stalls on `ResolvedRefs=False`, sync cert-manager-config, then
resync root-apps.

## Apps

| Application | Namespace | Environment |
|-------------|-----------|-------------|
| argocd | argocd | prd, sandbox |
| cert-manager | cert-manager | prd |
| cert-manager-config | cert-manager | prd |
| comfyui | comfyui | prd |
| external-secrets (eso) | external-secrets | prd, sandbox |
| external-dns | external-dns | prd, sandbox |
| gateway | gateway-system | prd, sandbox |
| gpu-switch | gpu-switch | prd |
| headlamp | headlamp | prd |
| homepage | homepage | prd, sandbox |
| lemonade-server | lemonade-server | prd |
| monitoring | monitoring (argocd in prd) | prd, sandbox |
| ollama | ollama | prd |
| open-webui | open-webui | prd |
| reloader | reloader | prd |

Sandbox is HTTP-only without cert-manager. It uses the `kubernetes-sandbox`
OpenBao mount and manages `sandbox.butaco.net.` through PowerDNS.
