# ADR-0026: Inject in-cluster secrets with External Secrets Operator instead of the helm-secrets plugin

- **Status:** Accepted
- **Date:** 2026-07-25
- **Related:** [ADR-0012](0012-openbao-eso-cluster-rebuild-registration.md),
  [`k8s/eso/README.md`](../../k8s/eso/README.md),
  [`k8s/monitoring/README.md`](../../k8s/monitoring/README.md),
  [`k8s/cert-manager/README.md`](../../k8s/cert-manager/README.md)

## Context

Argo CD originally rendered charts whose values files were SOPS-encrypted, using
the **helm-secrets** plugin to decrypt them at render time. This kept the
existing SOPS + AGE workflow (already used by Ansible and Terragrunt) as the
single mechanism for every secret in the repo.

Argo CD does not ship helm-secrets. Making the plugin available to the
repo-server requires either **injecting the tooling into the container at
startup** or **maintaining a custom image / CMP sidecar**. Both were implemented
here, and both were abandoned (see *What was actually tried* below).

The recurring problems were the same in either shape:

- **The rendering toolchain became part of the platform's build surface.**
  Argo CD upgrades had to be validated against the injection script or the
  custom image, not just against Argo CD.
- **Failures surfaced as render-time errors inside the repo-server**, far from
  the application being deployed. Diagnosing "why did this app fail to sync"
  meant reading repo-server logs and reproducing plugin behaviour rather than
  looking at the workload. The debugging loop was slow and the failure mode was
  unintuitive.
- **The workarounds were brittle**, depending on the internal file layout of an
  upstream plugin rather than on a supported interface.
- The AGE key had to be present in the repo-server, widening where the master
  decryption key lives.

Meanwhile External Secrets Operator (ESO) was already deployed and registered
against OpenBao (ADR-0012), covering the same requirement through a different
mechanism — and doing so **outside** the rendering path.

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
Ansible `*.sops.yaml` and Terragrunt `*.enc.env` — where the decrypting process
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

Because the rendered manifests are ordinary YAML, Argo CD needs no extra
tooling, and a failure to obtain a secret is visible **on the resource that
needs it** — `kubectl describe externalsecret` — instead of in the logs of a
shared component. Authentication uses the ESO ServiceAccount via Kubernetes
auth, so no long-lived credential is stored in the repo either (ADR-0012).

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

That one line went through **five iterations in a single day** (2026-04-03),
cycling between `sed`, `awk`, and an inline `python3` script — each attempt
requiring a pod restart to find out whether rendering worked. The dependency was
not on a documented interface but on the **line layout of an upstream file**.

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

This is the supported extension point and removed the startup patching, but it
**pinned two upstream versions together** — the helmfile base image and the
`argocd-cmp-server` binary copied out of a specific Argo CD release. Every Argo
CD upgrade meant rebuilding and revalidating the image, and the CI workflow
itself needed its own iterations. The maintenance cost simply moved from pod
startup to the release pipeline.

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
  operators and in OpenBao (see [SOPS setup](../sops_presentation.md)).
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
- **Do not re-litigate this with a new plugin mechanism.** Both the injection and
  the CMP-sidecar shapes were built and run here; the cost was not in choosing
  between them but in *having decryption inside the rendering path at all*. A
  future proposal to put tooling back into the repo-server should supersede this
  ADR explicitly, with an argument for why the debugging and upgrade cost is
  different this time.
