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

The groundwork for driving this from anywhere already exists and was built for
other reasons:

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

What is missing is a UI and, more importantly, somewhere for the **exclusivity
rule** to live. `gpu-switch.sh` enforces it by construction; a UI that merely
exposes "scale this deployment" does not.

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
2. **The application is a real container image, built from source in this
   project and published to a registry** — not code injected into a stock image
   at runtime — and it is built from **its own source and nothing else**: no
   Kubernetes client library, no frontend framework, no build step for the
   browser assets. The source lives in this repository at
   `k8s/gpu-switch/app/`, and CI publishes an immutable version tag to
   `ghcr.io`. See *Backend*, *Frontend*, and *Building the image* below.
3. **Write access is scoped to the four workloads.** The ServiceAccount gets
   cluster-wide `list` on `deployments` — needed to discover them by label — and
   `patch` on `deployments/scale` only through per-namespace `Role`s with
   explicit `resourceNames`. It is not `cluster-admin`.
4. **Authentication is enforced at the Gateway**, by an Envoy Gateway
   `SecurityPolicy` with Basic Auth sourced from OpenBao through ESO, exactly as
   ADR-0009 established for the Longhorn UI. The htpasswd value must be `{SHA}`
   format.

The implementation language is **Go**, chosen for the artifact it produces
rather than for how the code reads: a static binary runs on
`distroless/static`, which has no OS package layer and therefore no stream of
base-image CVEs to rebuild against on an application that is otherwise finished.
That matters more here than usual because this process holds credentials that
can scale cluster workloads. Go is also already a first-class language in this
project — [`k8s/monitoring/dashboards`](../../k8s/monitoring/dashboards/) is a Go
module with its own CI job — so nothing new enters the toolchain.

The language is the least consequential part of this section. The commitments
that actually constrain the design are the two dependency rules below.

### Why a backend at all, and not an nginx-only proxy

[`k8s/pdns-ui`](../../k8s/pdns-ui/README.md) needs no application code: nginx
proxies to PowerDNS and injects a static API key. That shape does not work here.

The ordering requirement is the first reason: "scale all to 0, then scale one to
1" is a sequence of API calls with a dependency between them, which a reverse
proxy cannot express. Pushing it into client-side JavaScript would put the
invariant back in the browser, where a closed tab mid-sequence leaves the GPU
claimed by nothing or by two workloads.

The credential is the second. The projected ServiceAccount token rotates roughly
hourly, and nginx reads its configuration once at startup — so a proxy-only
design would need a long-lived token `Secret` created out of band (the manual
step ADR-0015 documents for Headlamp login) and would then hold a non-expiring
credential in the cluster. A process that reads
`/var/run/secrets/kubernetes.io/serviceaccount/token` per request gets rotation
for free.

### Backend: the Kubernetes API over the standard library, not a client library

The application talks to the API server with `net/http` and `encoding/json`. It
does **not** use `client-go`. Two requests are the entire surface:

```text
GET   /apis/apps/v1/deployments?labelSelector=homelab%2Fgpu-switchable%3Dtrue
PATCH /apis/apps/v1/namespaces/{ns}/deployments/{name}/scale
      Content-Type: application/merge-patch+json     {"spec":{"replicas":N}}
```

`client-go` would pull in a large dependency tree and, more importantly, **couple
this artifact to Kubernetes minor versions** — so upgrading k0s would create
pressure to rebuild and revalidate an application that has not changed. That is
the ADR-0026 failure mode in miniature: an image whose maintenance is driven by
another product's release cadence. Against two requests, the library buys
nothing that justifies it. The `go.mod` should have no `require` block at all.

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

The requirement is four tiles, a state readout, and a poll loop. A framework
would add a dependency stream to be tracked and rebuilt forever, against a page
that will not grow. It would also break a property
[`k8s/pdns-ui`](../../k8s/pdns-ui/README.md) established and documented
deliberately: no external scripts, styles, or fonts. A LAN troubleshooting tool
should not need the internet to render.

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

No container image is built **from this repository** today, and that is not an
accident: ADR-0026 removed a CMP image and its GitHub Actions workflow. That
decision must not be quietly reversed here, so the distinction is stated
explicitly.

The homelab does build one image, elsewhere: `comfyui-docker`, in a Forgejo
repository on the LAN, built by Forgejo Actions on the self-hosted runner from
[`ansible/roles/forgejo_runner`](../../ansible/roles/forgejo_runner/README.md)
and published to the LAN registry as
`forgejo.home.butaco.net/takanao/comfyui-docker:latest`. That placement was
forced by size, not chosen as policy: the image bakes PyTorch ROCm wheels and
runs to tens of gigabytes, which does not fit in the ~14 GB of free disk on a
GitHub-hosted runner — hence a self-hosted runner with a 20 GB BuildKit cache.
The constraint does not generalise, and in particular does not reach a Go binary
on `distroless/static`.

The image ADR-0026 abandoned was a **derivative of another product's release
artifacts** — a helmfile base image with the `argocd-cmp-server` binary copied
out of a pinned Argo CD release. Its cost was structural: every Argo CD upgrade
required rebuilding and revalidating the image, because the image existed only to
extend a component the project did not own.

The image here is **our own application**. Its only upstream coupling is a base
image tag, which is exactly the kind of dependency Renovate already bumps across
this repository. Nothing about upgrading Argo CD, Envoy Gateway, or k0s requires
rebuilding it. ADR-0026's prohibition — do not put tooling inside the Argo CD
rendering path — is untouched by this decision.

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

This preserves the property ADR-0006 valued when it routed Packer output through
S3: **build and deploy stay decoupled.** Publishing an image does not touch the
cluster; the cluster references a published, checksummed artifact.

#### The source lives in this repository, at `k8s/gpu-switch/app/`

The deciding factor is **supply-chain visibility for a credentialed
application**, not developer convenience.

Renovate reaches a dependency only where it can see it. With the `Dockerfile` in
this repository, the `FROM golang:…` and `FROM …/distroless/static` lines are
tracked by the dockerfile manager and arrive as ordinary bump PRs, and the
published tag in `values.yaml` is tracked by the docker datasource. In a Forgejo
repository neither is visible: `renovate.json` disables the LAN registry outright
("unreachable from Mend cloud"), and Mend never sees a repository that is not on
GitHub at all.

Having argued above that a minimal base image matters *because this process holds
credentials that can scale cluster workloads*, it would be incoherent to then put
that base image where nobody is told it needs updating. `comfyui-docker` already
lives with exactly this — its README instructs a human to edit the PyTorch
`--index-url` by hand — which is tolerable for an image whose GPU-driver skew a
human must judge anyway, and not tolerable as the general case.

Between this repository and a separate GitHub repository, the difference is
smaller than it first appears:

- **Atomic review of chart + app is not available either way.** With immutable
  version tags the chart cannot reference a tag that does not yet exist, so the
  sequence is always: merge the app change, cut a tag, let CI publish, then bump
  `values.yaml`. Two steps in one repo or two steps across two — same shape.
- **A separate repository duplicates operational setup**, if not code: its own
  Renovate onboarding, its own Discord webhook secret for the
  `notify-workflow-failure` convention, its own lint workflows and automerge
  rules. That is the same class of harm ADR-0006 cited when it folded the Packer
  repo in — there it was Terragrunt structure and SOPS, here it is CI
  convention — even though the code itself would not be duplicated.
- **The directory convention already exists.**
  [`k8s/monitoring/dashboards`](../../k8s/monitoring/dashboards/) is a Go module
  with its own `go.mod`, `Makefile`, and CI job, sitting inside the `k8s/` tree
  beside what it serves. `k8s/gpu-switch/app/` matches it. The objection that
  the dashboards module generates build-time artifacts while this one is a
  runtime service is real but thin: both are Go modules this repository's CI
  builds.

The choice is also **cheap to reverse** — moving the source out later is a
`git filter-repo` and a registry name change in one values file. A decision that
is cheap to undo should start from the simpler arrangement.

Two Renovate details follow from this and constrain the implementation:

- The image tag must live in `k8s/gpu-switch/chart/values.yaml`, **not** in a
  chart template: `renovate.json` lists `**/templates/**` under `ignorePaths`, so
  a reference written into `templates/deployment.yaml` would never be bumped.
- `minimumReleaseAge: 1 day` applies to our own published image too, so a fresh
  release waits a day before Renovate offers it. Either accept the delay, exempt
  this package with a `packageRule`, or bump the tag by hand at release time.

### The label is the source of truth

`gpu-switch.sh` currently hardcodes the namespace/deployment pairs. Introducing a
second list in the chart would create two places to update when a GPU workload is
added or removed.

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

`gpu-switch.sh` is not replaced. It is the path that works when the Gateway,
ESO, or the UI itself is broken, and it is what an operator already at a
terminal will reach for.

Its `status` subcommand reports the same three states as `GET /api/state`
(`running` / `starting` / `stopped`), from the same label query. The two
interfaces should stay consistent in vocabulary; where they differ is only in
what they let you do — the CLI can restart the running workload, the UI
deliberately cannot.

## Alternatives considered

- **Ship the application in a `ConfigMap` on a stock interpreter image**, as
  `pdns-ui` ships its vendored `index.html` — avoids reintroducing an image
  build entirely, which is why it was the first candidate. Rejected because the
  `pdns-ui` precedent does not transfer: that `ConfigMap` holds a *vendored
  third-party artifact* that is never edited here, pinned by SHA256 and checked
  by the `vendor-sync` job. Code that is actually developed in this project wants
  what a build gives it — dependency resolution, a compile or lint step that runs
  before deploy, and a reviewable diff that is not a YAML string blob. It would
  also silently constrain the language choice to whatever runtime the base image
  happens to carry. *Rejected.*
- **Build in Forgejo and publish to the LAN registry**, as `comfyui-docker`
  does — the existing practice in this homelab, reusing a runner and registry
  that already exist and adding no public supply-chain surface. Rejected because
  Renovate can see neither the `Dockerfile` nor the published tag there, leaving
  the base image of a credentialed application on manual watch, and because
  the LAN registry's untracked images end up on `:latest`, which puts the
  deployed version outside Git and makes rollback something other than a values
  change. The precedent does not carry: `comfyui-docker` is in Forgejo because a
  tens-of-gigabytes ROCm image cannot be built on a GitHub-hosted runner, a
  constraint absent for a ~20 MB Go image. *Rejected.*
- **A separate GitHub application repository** — enforces the
  infrastructure/application boundary rather than leaving it to convention, and
  would keep this repository free of a runtime artifact. Rejected because the
  boundary buys little here (the release sequence is identical either way) while
  the second repository duplicates Renovate onboarding, notification secrets,
  and lint conventions. *Rejected, cheaply reversible if this repository turns
  out to be the wrong home.*
- **Use `client-go` for the Kubernetes calls** — the obvious choice, and the one
  that would need no defending in most projects. It brings typed objects,
  retries, and informers. But this app issues two requests and needs none of
  that, while the library's version skew policy would tie the image's rebuild
  cadence to Kubernetes upgrades. *Rejected*, and worth re-examining only if the
  app ever needs to watch resources rather than poll them.
- **A framework-based frontend (React/Vite or similar)** — better ergonomics if
  the UI grows. It will not grow: the feature is four buttons and a status line,
  bounded by the number of GPU workloads. The cost is a bundler stage in the
  image build and a permanent JavaScript dependency stream. *Rejected.*
- **Use the existing Headlamp UI as-is** (scale each `Deployment` by hand from
  `headlamp.prd.butaco.net`) — costs nothing and works today, so it remains the
  fallback until this is built. But it is four manual operations with no
  exclusivity guarantee, and Headlamp's login is a `cluster-admin` token: a
  routine daily action would be performed with unrestricted cluster credentials.
  *Rejected as the primary interface.*
- **A Headlamp plugin providing a GPU Switch page** — the best UX, and it reuses
  a deployed component. Now that an image build is accepted, its packaging cost
  is no longer disqualifying on its own; what remains is that the plugin is
  coupled to Headlamp's plugin API and must be revalidated on Headlamp upgrades —
  precisely the failure mode ADR-0026 recorded — and that it inherits Headlamp's
  `cluster-admin` session rather than the scoped ServiceAccount above.
  *Rejected.*
- **A button on the Homepage dashboard** — Homepage is a read-only dashboard; its
  `customapi` widget issues GET requests for display and cannot perform an
  action. Not technically possible on its own. Homepage remains useful as the
  link to the new UI and, via `customapi` against `/api/state`, as a place to
  show which workload currently holds the GPU. *Rejected as the mechanism,
  retained as a surface.*
- **An Argo CD custom resource action or Application parameter override** — Argo
  CD actions operate on a single resource, so "start this one and stop the other
  three" cannot be expressed; and driving replicas through Argo CD contradicts
  the `ignoreDifferences` decision that deliberately removed replica count from
  GitOps ownership. *Rejected.*
- **A ConfigMap or CRD holding the desired workload, reconciled by a controller**
  — the most Kubernetes-native shape, and it would let the switch be made by
  editing one object in Headlamp. But it requires writing and operating a
  controller for a four-value enum, and the editing experience (typing a name
  into a YAML field) is worse than a page with four buttons. *Rejected as
  disproportionate.*

## Consequences

- A UI that can stop and start workloads becomes reachable from the LAN. Basic
  Auth at the Gateway is what stands between the network and the GPU workloads,
  so the `SecurityPolicy` is not optional and the route must never be exposed
  without it. Basic Auth was judged sufficient for this threat model — a
  LAN-reachable, single-operator homelab — matching ADR-0009 rather than
  introducing a second authentication mechanism.
- **This project resumes publishing a container image**, roughly three months
  after ADR-0026 removed the last one. The build must stay a build of our own
  source with an upstream base image and nothing else; an image that repackages
  another product's release artifacts is what ADR-0026 rejected, and that
  boundary should be defended in review.
- **The homelab now has two image build paths, and the split needs a stated
  criterion or it will be decided by whoever goes first.** Large images that
  cannot be built on a hosted runner — GPU/ROCm workloads like `comfyui-docker` —
  belong in Forgejo on the self-hosted runner. Small images belong in GitHub
  Actions publishing to `ghcr.io`, where Renovate can see the base image. Size
  and runner capacity are the criterion; convenience is not.
- **The no-dependency rule is the thing to hold in review**, not the language.
  A `require` line in `go.mod` or a `package.json` appearing later is not a
  detail — it reintroduces exactly the rebuild-and-revalidate cost this ADR and
  ADR-0026 were both written to avoid. Adding one should be an explicit decision
  with a reason, not a convenience taken during implementation.
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
