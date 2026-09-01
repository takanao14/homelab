# ADR-0026: Inject in-cluster secrets with External Secrets Operator instead of the helm-secrets plugin

- **Status:** Accepted
- **Date:** 2026-07-25
- **Related:** [ADR-0012](0012-openbao-eso-cluster-rebuild-registration.md),
  [`k8s/eso/README.md`](../../k8s/eso/README.md),
  [`k8s/monitoring/README.md`](../../k8s/monitoring/README.md),
  [`k8s/cert-manager/README.md`](../../k8s/cert-manager/README.md)

## Context

Argo CD originally used **helm-secrets** to decrypt SOPS values during chart
rendering, extending the Ansible and Terragrunt SOPS + AGE workflow to Kubernetes.

Argo CD does not ship helm-secrets. We tried both startup injection and a custom
CMP sidecar image, then abandoned both (see *What was actually tried*).

Both approaches had the same problems:

- Argo CD upgrades also required validating the injection or custom image.
- Failures appeared as remote repo-server render errors rather than workload status.
- Startup injection depended on the plugin's internal file layout.
- The AGE key had to be present in the repo-server, widening where the master
  decryption key lives.

ESO was already registered with OpenBao (ADR-0012) and resolves secrets outside
the rendering path.

## Decision

**Use ESO for all in-cluster secret material. Do not use the helm-secrets
plugin, and do not customise or inject tooling into the Argo CD repo-server.**

- Secrets consumed by workloads are declared as `ExternalSecret` resources and
  materialised into `Secret` objects by ESO, sourced from OpenBao via
  `ClusterSecretStore/openbao`.
- Chart values under `k8s/` contain **references to secrets, not secret values**;
  no SOPS-encrypted values files are rendered by Argo CD.
- Argo CD runs as the upstream chart, with **no plugin and no custom image**.

SOPS + AGE remains the mechanism for secrets consumed **outside** the cluster —
Ansible `*.sops.yaml` and Terragrunt `*.sops.env` — where the decrypting process
is the operator's own shell, not a long-running platform component.

### Why this avoids the problem rather than relocating it

The plugin approaches all placed decryption **at render time, inside the
repo-server**. ESO moves it to **runtime, inside a controller of its own**:

| | helm-secrets (plugin) | ESO |
|---|---|---|
| **When secrets are resolved** | while Argo CD renders the chart | after the manifest is applied |
| **What Argo CD handles** | ciphertext it must decrypt | plain YAML referencing a name |
| **Where failures appear** | repo-server render errors | `ExternalSecret` status on the object |
| **What must be installed in Argo CD** | sops / age / helm-secrets | nothing |
| **Where the AGE key must exist** | in the repo-server | not in the cluster at all |

Argo CD therefore renders ordinary YAML. Secret failures appear on the
`ExternalSecret` resource, and Kubernetes auth avoids repository credentials
(ADR-0012).

## What was actually tried

Both supported ways of running helm-secrets under Argo CD were built and run
before this decision. Neither was rejected on paper.

### 1. Startup injection into the repo-server (abandoned)

An init container installed the toolchain into a shared `emptyDir` on every pod
start. It also had to **patch the plugin's own manifest** to make it usable:

```sh
apk add --no-cache curl bash
# strip the `command:` entry from the plugin's own plugin.yaml
sed -i '/^command:/d' /custom-tools/helm-plugins/helm-secrets/plugin.yaml
```

That patch went through five iterations on 2026-04-03, each requiring a pod
restart. It depended on upstream file layout rather than a documented interface.

### 2. CMP sidecar with a purpose-built image (abandoned)

A Config Management Plugin image was built and published by a dedicated GitHub
Actions workflow:

```dockerfile
FROM ghcr.io/helmfile/helmfile:v1.5.0
COPY --from=quay.io/argoproj/argocd:v3.3.9 \
    /usr/local/bin/argocd-cmp-server /usr/local/bin/argocd-cmp-server
USER 999
ENTRYPOINT ["/usr/local/bin/argocd-cmp-server"]
```

This removed startup patching but coupled the helmfile base image to a specific
Argo CD binary. Every Argo CD upgrade required rebuilding and validating the image.

### 3. Removal (2026-05-03 → 2026-05-05)

Applications were migrated to native `helm.valueFiles` with secrets supplied by
ESO, then the plugin configuration, the repo-server `plugins` volume, and
finally the CMP image and its build workflow were deleted.

## Alternatives considered

- **Keep helm-secrets for a few remaining charts** — avoids a full migration,
  but retains the entire repo-server customisation cost for a shrinking number
  of consumers. *Rejected.*
- **Decrypt at commit time and store plain values** — removes the plugin
  entirely, but puts secret material in Git. *Rejected outright.*
- **Move all secrets to SOPS-in-Ansible and template them into the cluster** —
  keeps one mechanism, but reintroduces a push-based step outside GitOps and
  breaks cluster-rebuild automation. *Rejected.*

## Consequences

- Argo CD stays close to upstream. Upgrades no longer require validating a
  rendering toolchain.
- The AGE key is no longer needed inside the cluster; the master key stays with
  operators and in OpenBao.
- **ESO becomes a hard dependency for workload startup.** If ESO cannot
  authenticate to OpenBao, dependent workloads do not receive their `Secret`s.
  This is why a rebuilt cluster must be re-registered with OpenBao before its
  applications can sync — the procedure recorded in ADR-0012.
- Secret rotation changes shape: updating a value in OpenBao propagates through
  ESO, but running pods keep the old value until restarted. `reloader` covers
  this by restarting workloads when their `Secret` changes.
- Two secret mechanisms coexist by design, split by **who decrypts**:
  in-cluster consumers use ESO; operator-run tooling (Ansible, Terragrunt) uses
  SOPS + AGE. New secrets should follow that boundary rather than reintroducing
  encrypted values files under `k8s/`.
- Reintroducing repo-server decryption requires an explicit superseding ADR that
  addresses the observed debugging and upgrade costs.
