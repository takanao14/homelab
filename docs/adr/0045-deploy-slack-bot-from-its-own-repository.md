# ADR-0045: Deploy slack-bot from its own repository into prd only

- **Status:** Accepted
- **Date:** 2026-09-08
- **Related:** [ADR-0014](0014-argocd-app-of-apps-shared-helm-chart.md),
  [ADR-0026](0026-eso-over-helm-secrets-for-in-cluster-secrets.md),
  [ADR-0027](0027-gpu-workload-switching-web-ui.md),
  [ADR-0035](0035-observe-ix2106-dhcp-leases-from-rpi4.md),
  [`k8s/slack-bot/README.md`](../../k8s/slack-bot/README.md)

## Context

[takanao14/slack-bot](https://github.com/takanao14/slack-bot) renders Slack
channel messages and emojis to the LED matrix driven by a gRPC service on rpi3.
It already has its own CI, Renovate configuration, and release flow: a `v*` tag
publishes an immutable, publicly pullable image to `ghcr.io/takanao14/slack-bot`.
What it does not have is a permanent home in the fleet.

Its shape constrains the options. Socket Mode means the process dials out to
Slack and nothing connects to it. The only inbound listener is `/healthz` for a
liveness probe. It needs two secrets, and one out-of-cluster dependency reached
over plaintext gRPC on the LAN.

## Decision

**Run it as a normal Argo CD application under `k8s/slack-bot/`, referencing a
published image tag, with the source staying in its own repository, and enable
it in prd only.**

Four properties define the decision:

1. **The source stays where it is.** ADR-0027 kept `gpu-switch` in this
   repository so Renovate could see both the `Dockerfile` base images and the
   deployed tag. Only the second half of that argument applies here, and it is
   preserved: the tag lives in `k8s/slack-bot/chart/values.yaml`, so Renovate's
   docker datasource bumps it like any third-party image. The first half is
   already covered by the bot's own Renovate configuration. Importing the Go
   module would duplicate CI, lint, notification, and dependency setup to buy
   nothing.
2. **prd only, with no sandbox contract file.** Slack delivers each event to
   exactly one of an app's Socket Mode connections. A second deployment sharing
   the same app token would therefore take a random half of production traffic
   and render it. Sandbox can only host this app with a separate Slack app, so
   the environment is left out rather than stubbed as disabled.
3. **No inbound surface.** There is no Service, HTTPRoute, or NetworkPolicy —
   nothing routes to the pod, and the kubelet reaches `/healthz` directly.
   Adding a Service for symmetry would only create an address that must then be
   defended.
4. **Tokens come from OpenBao through ESO** (ADR-0026), under
   `secret/k8s/slack-bot/tokens` with a narrow `k8s-slack-bot` policy on the prd
   Kubernetes auth role. Reloader restarts the pod when the Secret changes.

The LED address is configured as an IP. The service is outside the cluster and
resolved on the LAN by a short hostname that cluster DNS cannot complete. An
unreachable LED service degrades the bot to logging a warning, so this stays a
plain outbound dependency rather than a startup precondition.

## Alternatives considered

- **Run it on rpi3 with Ansible and systemd**, as ADR-0035 does for the DHCP
  lease observer. It removes a network hop to the LED service, but rpi3 carries
  DHCP — a critical path deliberately kept small — and the project already
  publishes a container. *Rejected.*
- **Move the source into this repository** under the ADR-0027 pattern. The
  release sequence would be unchanged while duplicating an existing working CI
  setup. *Rejected; reversible.*
- **Deploy to sandbox as well.** Without a second Slack app it splits production
  events across clusters. *Rejected.*

## Consequences

- The cluster now tracks a release cadence owned by another repository. A change
  reaches prd in two steps: tag there, then a Renovate values bump here.
- The image must stay publicly pullable. Making the package private would
  require an `imagePullSecret` sourced from OpenBao.
- A new OpenBao path and policy exist; the tokens are seeded from the encrypted
  Ansible inventory, and the SOPS-encrypted `.env` in the bot's repository
  remains the original of those values.
- Restarts and rollouts are cheap, but an in-flight LED send holds shutdown for
  up to the operation timeout, so `terminationGracePeriodSeconds` is coupled to
  a value configured in the image.
