# ArgoCD

ArgoCD configuration for Kubernetes cluster management via GitOps. Supports
`prd` and `sandbox`.

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

ArgoCD is initially deployed using helmfile, and subsequently self-manages itself.

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

Two helmfile hooks guard this step: one interrupts the deployment if the target
cluster context is wrong, the other if ArgoCD already self-manages the release.
The second can be overridden with `ARGOCD_BOOTSTRAP_FORCE=1`, which should only
be needed when deliberately re-bootstrapping.

For sandbox, use `k8s/argocd/sandbox`. It intentionally exposes ArgoCD over
HTTP only and does not install cert-manager.

### Getting in on a fresh cluster

The chart creates `argocd-secret` with no `data` block, so `argocd-server` finds
no `admin.password`, generates one, and writes the plaintext to
`argocd-initial-admin-secret`. It does this only while `PasswordHash` is empty,
and never deletes the secret afterwards.

```bash
kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d
```

**That generated password is the only way in until ESO supplies the real one**,
and ESO cannot authenticate until the rebuilt cluster is re-registered with
OpenBao ([ADR-0012](../../docs/adr/0012-openbao-eso-cluster-rebuild-registration.md)).
So delete it last, in this order:

1. Re-register the cluster
   (`ops-openbao_register_cluster.yaml -e cluster=<env>`).
2. Wait for `kubectl -n argocd get externalsecret argocd-admin-password` to
   report `SecretSynced`.
3. Log in with the password from OpenBao.
4. Only then `kubectl -n argocd delete secret argocd-initial-admin-secret`.

Leaving it costs nothing functionally — ESO has overwritten `admin.password`, so
the value no longer authenticates, and a later regeneration updates the secret
in place rather than being blocked by it. Delete it anyway: it is a plaintext
credential with no remaining purpose, readable by anything with get access to
the namespace, and a stale-but-plausible password is exactly what turns a login
problem into five failed attempts and a lockout (see
[Rotating the password](#rotating-the-password)).

Deleting it is also not a one-way door. Dropping `admin.password` makes
`argocd-server` generate a fresh one and recreate the secret — useful as a
recovery path, though ESO overwrites it again at the next refresh:

```bash
kubectl -n argocd patch secret argocd-secret --type=json -p '[{"op":"remove","path":"/data/admin.password"}]'
```

### Changing values

After bootstrap, `values-common.yaml` and `<env>/values.yaml` are read by
ArgoCD straight from git. **Commit and push — that is the whole apply step.**
The `argocd` Application has `selfHeal: true` and syncs on its own.

Running `helmfile apply` instead fails with:

```
invalid ownership metadata; annotation validation error:
missing key "meta.helm.sh/release-name"
```

ArgoCD applies with `ServerSideApply=true` and does not write Helm's ownership
annotations, so every resource introduced by a chart version newer than the
bootstrap one exists without them. Helm refuses to adopt those resources into
its release. The Helm release record therefore stops at the bootstrap version
while the live cluster tracks the chart version in `apps/values.yaml`; this
divergence is expected, not a fault to repair. A fresh-cluster bootstrap is
unaffected because no conflicting resources exist yet.

## Secrets Management

All application secrets are managed via External Secrets Operator (ESO) backed by OpenBao — see `k8s/eso/` for the `ClusterSecretStore` configuration.

### Admin password

The local `admin` password is declared, not left at the value `argocd-server`
generates on first start. `chart/templates/admin-external-secret.yaml` merges
`admin.password` and `admin.passwordMtime` into `argocd-secret` from OpenBao.
Each environment reads its own path, so prd and sandbox have independent
passwords:

| Environment | OpenBao key |
|-------------|-------------|
| prd | `k8s/argocd/prd/admin` |
| sandbox | `k8s/argocd/sandbox/admin` |

Both keys hold two properties:

| Property | Value | Maintained by |
|----------|-------|---------------|
| `password` | bcrypt hash of the password — **never the plaintext** | operator |
| `mtime` | RFC3339 timestamp | Ansible, automatically |

Only the hash is written by hand. It lives in the `openbao_argocd_admin` list in
the SOPS-encrypted `ansible/inventories/homelab/group_vars/openbao.sops.yaml`,
and `ops-openbao_seed_secrets.yaml` pushes it. That playbook compares the hash
against what OpenBao already holds and writes only on a difference, stamping
`mtime` with the current UTC time as it does. Rotating a password therefore logs
out every admin session for that environment, and re-running the playbook for
any other reason changes nothing. See
[`ansible/roles/openbao/README.md`](../../ansible/roles/openbao/README.md) for
the entry format and the hash command.

Read access comes from the `k8s-argocd-{env}` policies, which
`ops-openbao_configure.yaml` generates per cluster so neither environment's ESO
can read the other's hash.

`argocd-server` watches `argocd-secret`, so a change propagates without a
restart. Argo CD rejects every session token issued before `mtime` — that is the
mechanism behind the logout above. `mtime` is optional as far as Argo CD is
concerned: an absent or unparseable value leaves `PasswordMtime` nil and simply
disables the check, and for the `admin` account a parse failure is swallowed
rather than raised.

Why this shape rather than `configs.secret.argocdServerAdminPassword`:

- That value would put the bcrypt hash in git, which ADR-0026 rules out for
  anything under `k8s/`. Argo CD reads these values files straight from git and
  cannot decrypt SOPS.
- The chart defaults `argocdServerAdminPasswordMtime` to `now()`. Argo CD
  re-renders on every reconcile, so an unpinned mtime produces a permanent
  diff and re-invalidates sessions on each sync.

`creationPolicy: Merge` is required — `Owner` would make ESO the sole author of
`argocd-secret` and drop `server.secretkey`, which `argocd-server` generates and
owns. Merge needs the Secret to exist first; the helmfile bootstrap creates it
long before `root-apps.yaml` is applied.

**No bootstrap deadlock.** Argo CD does not need `admin.password` to run — it is
only used for human login — so the fact that Argo CD deploys the ESO that
supplies it is not circular. On a fresh cluster the bootstrap password works
until ESO (wave 0) syncs, and the `argocd` app (wave 1) applies the
ExternalSecret afterwards. If ESO or OpenBao is unreachable the ExternalSecret
just retries; Argo CD assigns no health status to `ExternalSecret`, so the
`argocd` Application does not go Degraded and later waves are not blocked. If
`admin.password` is ever missing entirely, `argocd-server` regenerates it and
recreates `argocd-initial-admin-secret`, so there is no lockout.

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

Steps 1–2 must come before step 3. Pushing first only leaves the ExternalSecret
in `SecretSyncedError` until OpenBao has the value; it does not block Argo CD.

#### Rotating the password

Replace the hash in `openbao_argocd_admin` and re-run
`ops-openbao_seed_secrets.yaml`. Two things then delay or mask the change:

**ESO refreshes hourly.** `refreshInterval: 1h`, so a rotation lands up to an
hour later. Force it:

```bash
kubectl -n argocd annotate externalsecret argocd-admin-password force-sync=$(date +%s) --overwrite
```

Argo CD does not fight this — the annotation belongs to a different field
manager, and ServerSideApply diffing ignores fields Argo CD does not own. The
value is applied once `admin.passwordMtime` matches the seeding run:

```bash
kubectl -n argocd get secret argocd-secret -o jsonpath='{.data.admin\.passwordMtime}' | base64 -d
```

**Five failed logins lock the account for five minutes**, and during the lockout
Argo CD returns `Invalid username or password` without checking the password at
all — a correct password looks exactly like a wrong one. Worse, every attempt
made while locked out refreshes `LastFailed`, so retrying keeps extending the
window. Stop trying and read the logs to tell the two apart:

```bash
kubectl -n argocd logs deploy/argocd-server --since=10m | grep -E "failed login|too many failed"
```

| Log line | Meaning |
|----------|---------|
| `User admin failed login N time(s)` | password really was compared and did not match |
| `User admin had too many failed logins (5)` | locked out; the password was never checked |

The failure counter lives in Redis and disappears when the window expires, so
its absence is the definitive "not locked out" check:

```bash
P=$(kubectl -n argocd get secret argocd-redis -o jsonpath='{.data.auth}' | base64 -d); kubectl -n argocd exec deploy/argocd-redis -- sh -c "REDISCLI_AUTH='$P' redis-cli --scan --pattern 'login*'"
```

No output means no recorded failures. Defaults are `defaultMaxLoginFailures = 5`
and `defaultFailureWindow = 300` seconds.

If the password genuinely does not match, check the hash against it locally
before re-seeding — note that zsh expands `!` even inside double quotes, so
always single-quote the password when generating the hash:

```bash
printf 'admin:%s\n' '<hash>' > /tmp/h && htpasswd -bv /tmp/h admin '<password>'; rm -f /tmp/h
```

## HTTPRoute

ArgoCD is exposed via Gateway API HTTPRoute. The hostname is configured in each environment's `values.yaml` (`server.ingress.hostname`) and rendered by the shared `chart/` Helm chart.

The `argocd.yaml` Application uses multi-source:
1. Upstream `argo-cd` Helm chart
2. Values ref (this repo)
3. `k8s/argocd/chart` — renders HTTPRoute from values

## App of Apps

Each environment's `root-apps.yaml` points at the shared `apps/` chart with
`helm.valueFiles: [../<env>/apps-values.yaml]`. The chart renders one
Application per enabled app; per-app environment differences live in
`k8s/<app>/<env>/values.yaml` files referenced by the generated Applications
(see [ADR-0014](../../docs/adr/0014-argocd-app-of-apps-shared-helm-chart.md)).

To deploy an app to another environment:

1. Set `apps.<name>.enabled: true` in `k8s/argocd/<env>/apps-values.yaml`.
2. Add `k8s/<app>/<env>/values.yaml` if the app takes per-env values.

To add a new application, add a template to `apps/templates/` and a defaults
entry (enabled: false, wave) to `apps/values.yaml`. Keep upstream chart
coordinates in `apps/values.yaml` — Renovate ignores `**/templates/**`, and
its regex manager matches the `repoURL:` / `chart:` / `targetRevision:` key
order there.

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
| 2 | longhorn-ui | Behind the authenticated Gateway route (ADR-0009) |

Wave gating relies on the Application health check re-enabled in
`values-common.yaml`; automated syncs never retry the same revision, so
`root-apps` and the CRD-racing apps carry explicit retry policies.

Known caveat: the Gateway HTTPS listener (wave 0) references the wildcard TLS
Secret and ReferenceGrant created by cert-manager-config (wave 1). On a fresh
prd cluster the listener reports `ResolvedRefs=False` until wave 1 syncs; if
the gateway app sticks at Progressing/Degraded in wave 0, trigger a manual sync
of cert-manager-config to create the Secret, then re-sync root-apps.

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
| headlamp | headlamp | prd |
| homepage | homepage | prd, sandbox |
| lemonade-server | lemonade-server | prd |
| longhorn-ui | longhorn-system | sandbox only |
| monitoring | monitoring (argocd in prd) | prd, sandbox |
| ollama | ollama | prd |
| open-webui | open-webui | prd |
| reloader | reloader | prd |

Sandbox intentionally uses HTTP only. Its Gateway has no HTTPS listener, and
cert-manager is not installed. ESO uses the `kubernetes-sandbox` OpenBao auth
mount, while external-dns manages `sandbox.butaco.net.` through PowerDNS.
Sandbox homepage is exposed at `http://homepage.sandbox.butaco.net` and reuses
the production dashboard Secret paths for staging validation. The sandbox
Longhorn UI is exposed through an authenticated reverse proxy instead of
routing directly to `longhorn-frontend`.
