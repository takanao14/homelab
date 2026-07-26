# ADR-0027: GPU workload switching through an authenticated in-cluster web UI

- **Status:** Proposed
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
   at runtime. See *Building the image* below; **which repository holds the
   source is left open** and is the one part of this ADR still to be settled.
3. **Write access is scoped to the four workloads.** The ServiceAccount gets
   cluster-wide `list` on `deployments` — needed to discover them by label — and
   `patch` on `deployments/scale` only through per-namespace `Role`s with
   explicit `resourceNames`. It is not `cluster-admin`.
4. **Authentication is enforced at the Gateway**, by an Envoy Gateway
   `SecurityPolicy` with Basic Auth sourced from OpenBao through ESO, exactly as
   ADR-0009 established for the Longhorn UI. The htpasswd value must be `{SHA}`
   format.

The **implementation language is deliberately not fixed here.** It is an
implementation detail with no architectural consequence, to be chosen when the
app is written.

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

### Building the image

This repository currently builds no container images, and that is not an
accident: ADR-0026 removed a CMP image and its GitHub Actions workflow. That
decision must not be quietly reversed here, so the distinction is stated
explicitly.

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
- **`linux/amd64` only.** The GPU worker and the rest of the prd cluster are
  amd64; multi-arch builds would be cost without a consumer.
- The new workflow must be added to the `workflows:` list in
  [`notify-workflow-failure.yaml`](../../.github/workflows/notify-workflow-failure.yaml),
  or its failures are silent.

This preserves the property ADR-0006 valued when it routed Packer output through
S3: **build and deploy stay decoupled.** Publishing an image does not touch the
cluster; the cluster references a published, checksummed artifact.

#### Open question: which repository holds the application source

Not settled. The two candidates pull in opposite directions, and both have
precedent in this project.

| | This repository (`k8s/gpu-switch/app/`) | A separate application repository |
|---|---|---|
| **Precedent** | ADR-0006 consolidated the Packer build *into* the monorepo and deleted the standalone repo | ADR-0026 removed the last image build from here |
| **Change to app + chart** | one PR, versions reviewed together | two PRs, two repos to check out |
| **Version bump flow** | Renovate bumps the tag after a release is cut — same as any image, but the release is cut from this repo | identical, and the boundary is enforced rather than conventional |
| **CI surface** | this repo gains a build-and-publish job alongside its lint jobs | this repo keeps only lint jobs |
| **What the repo is** | an infrastructure repo that also ships one runtime artifact | infrastructure stays infrastructure |

The consolidation in ADR-0006 was driven by a specific harm — the standalone
Packer repo **duplicated this repo's Terragrunt structure, env directories, and
SOPS secrets**, so the two had to be kept in step by hand. A small application
repo duplicates none of that; it would share only a CI convention. So ADR-0006
is weaker precedent here than it first appears, and the decision rests instead on
whether this repository should be in the business of publishing runtime
artifacts at all.

Whichever is chosen, the deployment side is unchanged: a version tag in
`k8s/gpu-switch/chart/values.yaml`, bumped by Renovate.

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
