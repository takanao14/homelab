# ArgoCD

Argo CD GitOps configuration for `prd` and `sandbox`.

## Directory Structure

```
argocd/
├── values-common.yaml        # Common Helm values (insecure mode, ESO defaults)
├── chart/                    # Shared Helm chart for ArgoCD HTTPRoute + admin password
│   └── templates/
│       ├── httproute.yaml    # Uses server.ingress.hostname from values
│       └── admin-external-secret.yaml  # Merges admin.password into argocd-secret
├── apps/                     # App-of-apps chart: one Application template per app
│   ├── Chart.yaml
│   ├── values.yaml           # All apps disabled by default; waves; upstream chart versions
│   └── templates/            # Gated by apps.<name>.enabled, env via {{ .Values.env }}
├── prd/
│   ├── helmfile.yaml         # Initial deployment config for prd
│   ├── values.yaml           # Route hostname + openbao.adminPassword.key for prd
│   ├── apps-values.yaml      # env: prd + enabled apps
│   └── root-apps.yaml        # Bootstrap App of Apps for prd
└── sandbox/
    ├── helmfile.yaml         # Initial deployment config for sandbox
    ├── values.yaml           # HTTP route + openbao.adminPassword.key for sandbox
    ├── apps-values.yaml      # env: sandbox + enabled apps
    └── root-apps.yaml        # Bootstrap App of Apps for sandbox
```

## Environments

| Environment | Cluster | ArgoCD URL |
|-------------|---------|------------|
| prd | prd-homelab | `argocd.prd.butaco.net` |
| sandbox | sandbox-homelab | `http://argocd.sandbox.butaco.net` |

> `butaco.net` is a personal domain. Replace with your own domain in `prd/values.yaml` and `sandbox/values.yaml`.

## Initial Deployment

Helmfile bootstraps Argo CD; App of Apps then takes ownership.

> **helmfile is for bootstrap only — never run `helmfile apply` against a
> cluster that already runs ArgoCD.** Once `root-apps.yaml` is applied, the
> `argocd` Application takes over this release and re-running helmfile fails
> (see [Changing values](#changing-values) below).

```bash
# prd environment
cd k8s/argocd/prd
helmfile apply

# Apply root App of Apps
kubectl apply -f k8s/argocd/prd/root-apps.yaml
```

Hooks reject the wrong context or an already self-managed release. Use
`ARGOCD_BOOTSTRAP_FORCE=1` only for deliberate re-bootstrap.

For sandbox, use `k8s/argocd/sandbox`. It intentionally exposes ArgoCD over
HTTP only and does not install cert-manager.

### Getting in on a fresh cluster

Without `admin.password`, the server generates a bootstrap password in
`argocd-initial-admin-secret` and does not later delete it.

```bash
kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d
```

It is the only login until ESO syncs after OpenBao cluster registration
([ADR-0012](../../docs/adr/0012-openbao-eso-cluster-rebuild-registration.md)).
Delete it last:

1. Re-register the cluster
   (`ops-openbao_register_cluster.yaml -e cluster=<env>`).
2. Wait for `kubectl -n argocd get externalsecret argocd-admin-password` to
   report `SecretSynced`.
3. Log in with the password from OpenBao.
4. Only then `kubectl -n argocd delete secret argocd-initial-admin-secret`.

After ESO overwrites the hash, the bootstrap value no longer authenticates, but
delete the stale plaintext credential to prevent misleading login attempts.

For recovery, removing `admin.password` regenerates the bootstrap secret until
ESO overwrites it:

```bash
kubectl -n argocd patch secret argocd-secret --type=json -p '[{"op":"remove","path":"/data/admin.password"}]'
```

### Changing values

After bootstrap, commit and push value changes; `selfHeal` applies them.

Running `helmfile apply` instead fails with:

```
invalid ownership metadata; annotation validation error:
missing key "meta.helm.sh/release-name"
```

Server-side apply creates later resources without Helm ownership annotations,
so Helm cannot adopt them. The Helm release remains at its bootstrap version
while Git tracks the live chart version; this divergence is expected.

## Secrets Management

ESO supplies application secrets from OpenBao; see `k8s/eso/`.

### Admin password

ESO merges each environment's OpenBao-backed `admin.password` and
`admin.passwordMtime` into `argocd-secret`:

| Environment | OpenBao key |
|-------------|-------------|
| prd | `k8s/argocd/prd/admin` |
| sandbox | `k8s/argocd/sandbox/admin` |

Both keys hold two properties:

| Property | Value | Maintained by |
|----------|-------|---------------|
| `password` | bcrypt hash of the password — **never the plaintext** | operator |
| `mtime` | RFC3339 timestamp | Ansible, automatically |

Store only the hash in the SOPS-encrypted `openbao_argocd_admin` list.
`ops-openbao_seed_secrets.yaml` writes changes and stamps UTC `mtime`, logging
out existing sessions only on rotation. See
[`ansible/roles/openbao/README.md`](../../ansible/roles/openbao/README.md) for
the entry format and the hash command.

Per-cluster `k8s-argocd-{env}` policies isolate hash access.

The server watches the Secret and rejects sessions older than `mtime` without a
restart. Missing or invalid `mtime` disables that cutoff.

Why this shape rather than `configs.secret.argocdServerAdminPassword`:

- It exposes the bcrypt hash in Git, prohibited by ADR-0026.
- Its default `now()` mtime creates permanent drift and repeated logout.

`creationPolicy: Merge` preserves the server-owned `server.secretkey`; helmfile
creates the Secret before the ExternalSecret.

There is no bootstrap deadlock: the password is only for human login, failed
ExternalSecrets retry without blocking waves, and the server regenerates a
missing bootstrap password.

#### Rollout

1. Add the two `openbao_argocd_admin` entries — `env` plus the bcrypt hash
   (`sops ansible/inventories/homelab/group_vars/openbao.sops.yaml`).
2. Seed them and grant read access:

   ```bash
   ansible-playbook ansible/playbooks/ops-openbao_seed_secrets.yaml
   ansible-playbook ansible/playbooks/ops-openbao_configure.yaml
   ```

   `ops-openbao_configure.yaml` writes the `k8s-argocd-{env}` policies but does
   **not** rebind the ESO role. Run the registration playbook per cluster so
   `k8s-eso` picks up the new policy:

   ```bash
   ansible-playbook ansible/playbooks/ops-openbao_register_cluster.yaml -e cluster=prd
   ansible-playbook ansible/playbooks/ops-openbao_register_cluster.yaml -e cluster=sandbox
   ```

3. Commit and push — the `argocd` Application syncs on its own.
4. Confirm the ExternalSecret resolved:
   `kubectl -n argocd get externalsecret argocd-admin-password`
5. Log in with the new password. **Only then** delete the now-stale
   `argocd-initial-admin-secret` — it is the fallback if step 4 failed.

Complete OpenBao steps before pushing; otherwise the ExternalSecret waits in
`SecretSyncedError` without blocking Argo CD.

#### Rotating the password

Replace the hash and rerun `ops-openbao_seed_secrets.yaml`. Two behaviors can
delay or mask rotation:

**Hourly ESO refresh.** Force immediate synchronization with:

```bash
kubectl -n argocd annotate externalsecret argocd-admin-password force-sync=$(date +%s) --overwrite
```

Confirm `admin.passwordMtime` matches the seeding run:

```bash
kubectl -n argocd get secret argocd-secret -o jsonpath='{.data.admin\.passwordMtime}' | base64 -d
```

**Five failures lock the account for five minutes.** Attempts during lockout
extend it and return the same error as a wrong password; stop and inspect logs:

```bash
kubectl -n argocd logs deploy/argocd-server --since=10m | grep -E "failed login|too many failed"
```

| Log line | Meaning |
|----------|---------|
| `User admin failed login N time(s)` | password really was compared and did not match |
| `User admin had too many failed logins (5)` | locked out; the password was never checked |

No Redis login key means the lockout has expired:

```bash
P=$(kubectl -n argocd get secret argocd-redis -o jsonpath='{.data.auth}' | base64 -d); kubectl -n argocd exec deploy/argocd-redis -- sh -c "REDISCLI_AUTH='$P' redis-cli --scan --pattern 'login*'"
```

Defaults are five failures and 300 seconds.

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

Each root points at the shared `apps/` chart and its environment values. The
chart renders enabled Applications and their per-app overrides (ADR-0014).

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
