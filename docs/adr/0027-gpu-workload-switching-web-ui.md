# ADR-0027: GPU workload switching through an authenticated in-cluster web UI

- **Status:** Accepted
- **Date:** 2026-07-26
- **Related:** [ADR-0006](0006-custom-image-pipeline-monorepo-and-seaweedfs-s3.md),
  [ADR-0009](0009-longhorn-ui-exposed-through-authenticated-gateway-route.md),
  [ADR-0015](0015-headlamp-per-cluster-in-cluster-deployment.md),
  [ADR-0019](0019-merge-gpu-worker-into-prd-retire-dev-cluster.md),
  [ADR-0026](0026-eso-over-helm-secrets-for-in-cluster-secrets.md),
  [`k8s/pdns-ui/README.md`](../../k8s/pdns-ui/README.md),
  [`scripts/README.md`](../../scripts/README.md)

## Context

The prd cluster has exactly one AMD GPU (`amd.com/gpu: "1"`), and four
workloads compete for it: `ollama`, `comfyui`, `lemonade-server`, and `vllm`.
Only one may run at a time; a second one scheduled against the same device stays
`Pending` forever.

Switching is currently a workstation-only operation via
[`scripts/gpu-switch.sh`](../../scripts/gpu-switch.sh), which scales every GPU
deployment to 0 and then the selected one to 1. It requires a kubeconfig, a
checkout of this repository, and a shell — so the switch cannot be made from a
phone or from a machine that is not set up as an operator workstation.

Existing controls already support remote switching:

- Every GPU workload's `Deployment`, `PersistentVolumeClaim`, and `HTTPRoute`
  carries `homelab/gpu-switchable: "true"`.
- The four Argo CD `Application`s declare
  `ignoreDifferences` on `/spec/replicas` with `RespectIgnoreDifferences=true`,
  so **replica count is deliberately outside GitOps ownership** and a manual
  scale is not reverted by self-heal.
- The custom health Lua in
  [`k8s/argocd/values-common.yaml`](../../k8s/argocd/values-common.yaml) reads
  that label to report a scaled-to-zero workload — and its unbound PVC and
  backend-less HTTPRoute — as `Healthy` rather than `Degraded`.

The missing pieces are a UI and a server-side exclusivity rule equivalent to
`gpu-switch.sh`.

## Decision

**Add a purpose-built web UI as a normal Argo CD-managed app under
`k8s/gpu-switch/`, running from a container image built and published by CI,
exposed through the shared Envoy Gateway behind Basic Auth, with the
`homelab/gpu-switchable` label as the single source of truth for which workloads
are switchable.**

```text
browser ──https──► shared-gateway-envoy ──► gpu-switch pod
                   (SecurityPolicy           ├─ GET  /api/state   list Deployments by label
                    Basic Auth)              └─ POST /api/switch  scale all to 0, then target to 1
                                                      │
                                                      └─► kubernetes.default.svc (ServiceAccount)
```

Four properties define the decision:

1. **The exclusivity rule lives server-side.** `POST /api/switch` performs the
   whole "all down, then one up" sequence. The browser chooses a target, never a
   replica count, so the invariant cannot be violated by a partial interaction.
2. **Build the project-owned source as a container image.** Source lives at
   `k8s/gpu-switch/app/`; CI publishes immutable tags to `ghcr.io`. It uses no
   Kubernetes client library, frontend framework, or browser build step.
3. **Write access is scoped to the four workloads.** The ServiceAccount gets
   cluster-wide `list` on `deployments` — needed to discover them by label — and
   `patch` on `deployments/scale` only through per-namespace `Role`s with
   explicit `resourceNames`. It is not `cluster-admin`.
4. **Authentication is enforced at the Gateway**, by an Envoy Gateway
   `SecurityPolicy` with Basic Auth sourced from OpenBao through ESO, exactly as
   ADR-0009 established for the Longhorn UI. The htpasswd value must be `{SHA}`
   format.

Use **Go** for a static binary on `distroless/static`, minimizing the runtime
surface of a credentialed process. Go already exists in the project toolchain.

### Why a backend at all, and not an nginx-only proxy

[`k8s/pdns-ui`](../../k8s/pdns-ui/README.md) needs no application code: nginx
proxies to PowerDNS and injects a static API key. That shape does not work here.

The required "scale all to 0, then one to 1" sequence cannot be expressed by a
proxy and must not depend on a browser completing multiple calls.

A backend can also read the rotating projected ServiceAccount token per request;
nginx configuration would otherwise require a long-lived token Secret.

### Backend: the Kubernetes API over the standard library, not a client library

The application talks to the API server with `net/http` and `encoding/json`. It
does **not** use `client-go`. Two requests are the entire surface:

```text
GET   /apis/apps/v1/deployments?labelSelector=homelab%2Fgpu-switchable%3Dtrue
PATCH /apis/apps/v1/namespaces/{ns}/deployments/{name}/scale
      Content-Type: application/merge-patch+json     {"spec":{"replicas":N}}
```

For two requests, `client-go` adds an unjustified dependency tree and Kubernetes
version coupling. `go.mod` therefore has no `require` block.

Credentials come from `/var/run/secrets/kubernetes.io/serviceaccount/{token,ca.crt}`
and are re-read rather than cached for the process lifetime — this is the token
rotation argument from the previous section, made concrete.

Four further properties are load-bearing:

- **`/healthz` must not touch the API server.** If liveness and readiness depend
  on apiserver reachability, a transient control-plane blip restarts the pod and
  removes it from the Gateway. Process health and cluster reachability are
  different questions; the latter surfaces as an error from `/api/state`, where
  a human can see it.
- **`POST /api/switch` returns once the scale calls are issued, not when the new
  pod is ready.** Model loading for `vllm` and `ollama` runs into minutes, past
  any reasonable HTTP timeout and past Envoy's. Progress is observed by polling
  `/api/state`, which is why `starting` is a first-class state rather than a
  transient the UI can ignore.
- **The client sends a workload name, never a namespace.** The namespace is
  resolved server-side from the labelled set. Accepting a namespace would turn
  the app into a general-purpose scaler for everything its RBAC permits; RBAC is
  the backstop, not the design.
- **Switches are serialised by an in-process mutex, with the Deployment at
  `replicas: 1`.** This is explicitly *not* a cluster-wide lock — `gpu-switch.sh`
  can still act concurrently. The hard constraint is enforced by the device
  plugin: only one pod is ever allocated the single `amd.com/gpu`, and the loser
  stays `Pending`. The application makes the common case correct instead of
  reimplementing a lock it cannot own.

Basic Auth at the Gateway means the browser attaches credentials to *any*
request to this origin, so a cross-origin form POST could trigger a switch. The
worst outcome is an unwanted GPU switch, but the mitigation is a few lines:
require `Content-Type: application/json` (which a simple cross-origin form
cannot send, forcing a preflight that fails) and check
`Sec-Fetch-Site: same-origin`.

### Frontend: no framework and no build step

The page is one `index.html` with inline CSS and plain JavaScript, embedded in
the binary with `embed.FS`. There is no npm, no bundler, and no second build
stage.

Four tiles, state, and polling do not justify a framework. The page uses no
external scripts, styles, or fonts and renders without internet access.

The interaction rules that follow from what this actually does:

- **Confirm before switching.** A switch stops whatever is running — an in-flight
  inference or a ComfyUI render — and the stated motivation for this UI is using
  it from a phone, where a mis-tap is easy. A two-step tile-then-confirm beats a
  native `confirm()` dialog on mobile.
- **The running workload's tile is inert.** Re-selecting it would otherwise scale
  it to 0 and back to 1, restarting it for nothing. Making the tile
  unclickable removes that footgun without changing `gpu-switch.sh`'s semantics,
  where re-selecting the running workload remains a deliberate way to restart it.
- **Polling pauses when the page is hidden** (`document.hidden`), with an
  immediate fetch on return. A phone left on this page should not poll all night.
- **`starting` shows elapsed time.** Minutes of silence look like a failure.
- Mobile first: single column, large tap targets, nothing that depends on hover.

If the UI links to each workload's own hostname, those hostnames come from chart
values as environment variables — reading `HTTPRoute` objects would widen RBAC
for a cosmetic feature.

### Building the image

ADR-0026 removed a CMP image from this repository. This image differs because it
packages project-owned application source rather than another product's tooling.

`comfyui-docker` remains in Forgejo because its multi-gigabyte ROCm build needs a
self-hosted runner. That capacity constraint does not apply to this small Go image.

Its only upstream coupling is a Renovate-managed base image. Argo CD, Envoy
Gateway, and k0s upgrades do not require rebuilding it, and it does not enter
Argo CD's rendering path.

Shipping the code in a `ConfigMap` on a stock interpreter image, as `pdns-ui`
does with its vendored `index.html`, was considered and rejected; see
*Alternatives considered*.

Concretely:

- **Registry: `ghcr.io`.** The project is already on GitHub with GitHub Actions
  CI, so publishing uses the built-in `GITHUB_TOKEN` with `packages: write` and
  introduces no new credential. A public package needs no `imagePullSecret`; a
  private one would need one via ESO and OpenBao. Standing up a self-hosted
  registry is not justified for one image — SeaweedFS (ADR-0006) serves VM
  images over S3 and is not a container registry.
- **Tag-driven releases.** CI publishes an immutable version tag on a git tag,
  not a floating tag on every push. The chart then references a fixed version,
  and Renovate's docker datasource bumps `k8s/gpu-switch/chart/values.yaml` the
  same way it bumps every third-party image — the deployed version is visible in
  git, and rollback is a value change.
- **Multi-stage build onto `distroless/static:nonroot`.** The runtime stage
  holds the binary and nothing else — the browser assets are inside it via
  `embed.FS`, and the API server's CA comes from the ServiceAccount mount, not
  from a system trust store. The pod then runs `readOnlyRootFilesystem`,
  non-root, with all capabilities dropped and no writable volume.
- **`linux/amd64` only.** The GPU worker and the rest of the prd cluster are
  amd64; multi-arch builds would be cost without a consumer.
- The new workflow must be added to the `workflows:` list in
  [`notify-workflow-failure.yaml`](../../.github/workflows/notify-workflow-failure.yaml),
  or its failures are silent.

Build and deploy remain decoupled: publishing does not change the cluster.

#### The source lives in this repository, at `k8s/gpu-switch/app/`

The source stays here for supply-chain visibility.

Here Renovate sees both `Dockerfile` base images and the deployed tag. It cannot
see the Forgejo repository or LAN registry, which would make updates manual.

Compared with a separate GitHub repository:

- Immutable tags already require merge, publish, then values bump in either layout.
- A separate repository duplicates Renovate, notifications, lint, and automerge setup.
- `k8s/monitoring/dashboards` already establishes an in-tree Go module convention.

Moving the source later remains cheap, so start with the simpler layout.

Two Renovate details follow from this and constrain the implementation:

- The image tag must live in `k8s/gpu-switch/chart/values.yaml`, **not** in a
  chart template: `renovate.json` lists `**/templates/**` under `ignorePaths`, so
  a reference written into `templates/deployment.yaml` would never be bumped.
- `minimumReleaseAge: 1 day` applies to our own published image too, so a fresh
  release waits a day before Renovate offers it. Either accept the delay, exempt
  this package with a `packageRule`, or bump the tag by hand at release time.

### The label is the source of truth

`gpu-switch.sh` currently hardcodes namespace/deployment pairs, which would
duplicate a chart list.

Since `homelab/gpu-switchable` is already present on every GPU workload and is
already load-bearing for Argo CD health, **discovery moves to the label on both
paths**: the script switches to
`kubectl get deploy -A -l homelab/gpu-switchable=true`, and the UI lists by the
same selector. Adding a workload then means adding the label.

RBAC is the one place that stays an explicit list: `resourceNames` cannot be
expressed as a label selector, so the chart's values enumerate the four
namespace/name pairs to generate the `Role`s. This is accepted — an authorization
boundary should be explicit, and the failure mode of forgetting it is a denied
write, not a silently broken invariant.

### The CLI stays

Keep `gpu-switch.sh` as the terminal and recovery path when the UI stack is down.

Its `status` subcommand reports the same three states as `GET /api/state`
(`running` / `starting` / `stopped`), from the same label query. The two
interfaces should stay consistent in vocabulary; where they differ is only in
what they let you do — the CLI can restart the running workload, the UI
deliberately cannot.

## Alternatives considered

- **Application source in a `ConfigMap` on a stock interpreter.** Unlike
  `pdns-ui`'s pinned vendor artifact, project-owned code benefits from compile,
  lint, and normal source diffs. *Rejected.*
- **Build in Forgejo and publish to the LAN registry.** Renovate cannot see the
  source or tag there, and the small image needs no self-hosted runner. *Rejected.*
- **A separate GitHub repository.** The release sequence is unchanged, while
  Renovate, notification, and lint setup are duplicated. *Rejected; reversible.*
- **Use `client-go`.** Two requests do not justify its dependency tree and
  Kubernetes version coupling. Reconsider if polling becomes watching. *Rejected.*
- **A frontend framework.** Four buttons and status do not justify a bundler or
  JavaScript dependency stream. *Rejected.*
- **Use Headlamp as-is.** It requires four unrestricted `cluster-admin` actions
  and does not enforce exclusivity. *Rejected as the primary interface.*
- **A Headlamp plugin.** It couples upgrades to Headlamp's plugin API and
  inherits its `cluster-admin` session. *Rejected.*
- **A Homepage button.** Homepage is read-only; retain it only as a link/status
  surface. *Rejected as the mechanism.*
- **An Argo CD action or parameter override.** Actions cannot atomically switch
  multiple resources, and replicas are intentionally outside GitOps. *Rejected.*
- **A desired-workload ConfigMap/CRD and controller.** Operating a controller
  for a four-value choice is disproportionate. *Rejected.*

## Consequences

- The LAN-reachable route requires Gateway `SecurityPolicy`; Basic Auth matches
  the single-operator threat model from ADR-0009.
- The project resumes publishing an image, limited to project-owned source and
  upstream base images; repackaging product artifacts remains out of scope.
- Large GPU images use Forgejo/self-hosted runners; small Renovate-visible images
  use GitHub Actions and `ghcr.io`.
- New Go or frontend dependencies require explicit justification in review.
- The UI cannot restart the workload that is already running, because its tile is
  inert by design. `./gpu-switch.sh <name>` remains the way to do that — one more
  reason the CLI is not redundant.
- A new OpenBao path `secret/k8s/gpu-switch/basic-auth` is required, seeded from
  the encrypted Ansible `openbao_secrets` inventory, together with a narrow
  policy for it in the prd Kubernetes auth role.
- Switching from the UI is invisible to Git. That is already true of
  `gpu-switch.sh` and is intended: replica count is runtime state, not desired
  state. Anyone reading the repository to learn which GPU workload is running
  will not find the answer there — `kubectl get deploy -A -l
  homelab/gpu-switchable=true` is the answer.
- Adding a GPU workload requires the label (for discovery and Argo CD health), an
  entry in the `gpu-switch` chart values (for RBAC), and the `ignoreDifferences`
  block on its Application. Forgetting the label hides it from both the CLI and
  the UI; forgetting the RBAC entry makes the switch fail with a 403.
- The UI has a dependency the CLI does not: it can only be reached when the
  Gateway, ESO, and the app itself are healthy. Keeping `gpu-switch.sh` working
  is therefore a requirement, not a courtesy — including after it moves to
  label-based discovery.
